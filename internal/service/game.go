package service

import (
	"cmp"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	mrand "math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Cheyzie/pav_game/internal/dtos"
	"github.com/Cheyzie/pav_game/internal/model"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const codeCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1
const maxMessageRunes = 500
const maxRounds = 8

var (
	ErrRoomCodeGenerate error = errors.New("generate room code err")
	ErrTokenGenerate    error = errors.New("generate token err")
	ErrRoomNotExists    error = errors.New("room not exists")
	ErrPlayerNotExists  error = errors.New("player not exists")
	ErrNicknameTaken    error = errors.New("nickname has been already taken")
)

type GameRepository interface {
	Store(room *model.Game) error
}

type ActionDispatcher func(
	s *GameService,
	room *model.Room,
	player *model.Player,
	payload *string,
) (*model.GameState, error)

type PromptRepository interface {
	Store(room *model.Prompt) error
	GetRand(usedID []uint) (model.Prompt, error)
}

type GameService struct {
	rooms       map[string]*model.Room
	userRooms   map[uint]*model.Room
	gameRepo    GameRepository
	promptRepo  PromptRepository
	dispatchers map[dtos.ActionType]ActionDispatcher
	mu          sync.Mutex
}

func NewGameService(gameRepo GameRepository, promptRepo PromptRepository) *GameService {
	return &GameService{
		rooms:      make(map[string]*model.Room),
		userRooms:  make(map[uint]*model.Room),
		gameRepo:   gameRepo,
		promptRepo: promptRepo,
		dispatchers: map[dtos.ActionType]ActionDispatcher{
			dtos.SendMessageAction: dispatchMessage,
			dtos.ReadyAction:       dispatchReady,
			dtos.LieAction:         dispatchLie,
			dtos.VoteAction:        dispatchVote,
			dtos.ResetAction:       dispatchRestart,
		},
		mu: sync.Mutex{},
	}
}

func (s *GameService) CreateRoom(userID uint) (*model.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if room, ok := s.userRooms[userID]; ok {
		return room, nil
	}
	code, err := s.generateCode()
	if err != nil {
		log.Printf("generate room code err: %s\n", err.Error())
		return nil, ErrRoomCodeGenerate
	}
	statesChan := make(chan model.GameState, 5)
	messagesChan := make(chan []byte, 25)

	room := &model.Room{
		Code:            code,
		HostID:          userID,
		Round:           1,
		State:           model.LobbyState,
		UsedPromptIDs:   make([]uint, 0),
		Timers:          map[model.GameState][]*time.Timer{},
		StateTransition: statesChan,
		Messages:        messagesChan,
		Done:            make(chan struct{}),
	}
	go func() {
		for {
			select {
			case <-room.Done:
				return
			case message := <-messagesChan:
				s.mu.Lock()
				buses := make([]chan<- []byte, 0, len(room.Players))
				for _, p := range room.Players {
					if p.MessageBus != nil {
						buses = append(buses, p.MessageBus)
					}
				}
				s.mu.Unlock()

				for _, bus := range buses {
					select {
					case bus <- message:
					default: // slow client: drop rather than stall the room
					}
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-room.Done:
				return
			case nextState := <-statesChan:
				switch nextState {
				case model.LobbyState:
					s.mu.Lock()

					room.State = nextState
					for _, timer := range room.Timers[model.LobbyState] {
						timer.Stop()
					}
					room.Timers[model.LobbyState] = nil
					s.resetPlayers(room)
					room.Round += 1
					room.PhaseEndsAt = time.Time{}
					s.broadcastRoom(room, map[string]any{
						"type": "game_waiting",
					})
					s.mu.Unlock()
				case model.LieState:
					s.mu.Lock()
					if room.State != model.LobbyState {
						s.mu.Unlock()
						continue
					}
					s.mu.Unlock()
					prompt, err := s.promptRepo.GetRand(room.UsedPromptIDs)
					if err != nil {
						log.Printf("get prompt err: %s\n", err.Error())
						s.mu.Lock()
						s.resetPlayers(room)
						timer := time.AfterFunc(5*time.Second, func() {
							room.StateTransition <- model.LobbyState
						})
						room.Timers[model.LobbyState] = append(room.Timers[model.LobbyState], timer)
						s.mu.Unlock()
						continue
					}
					room.UsedPromptIDs = append(room.UsedPromptIDs, prompt.ID)
					s.mu.Lock()
					room.State = model.LieState
					room.Prompt = &prompt.Situation
					truth := strings.ToLower(strings.TrimSpace(prompt.Truth))
					room.Truth = &truth
					room.Answers = nil
					room.PhaseEndsAt = time.Now().Add(60 * time.Second)
					s.mu.Unlock()
					s.broadcastRoom(room, map[string]any{
						"type": "game_started",
						"payload": map[string]any{
							"prompt": room.Prompt,
						},
					})
					timer := time.AfterFunc(60*time.Second, func() {
						room.StateTransition <- model.VotingState
					})
					s.mu.Lock()
					room.Timers[model.VotingState] = append(
						room.Timers[model.VotingState],
						timer,
					)
					s.mu.Unlock()
				case model.VotingState:
					s.mu.Lock()
					if room.State != model.LieState {
						s.mu.Unlock()
						continue
					}
					room.State = model.VotingState
					for _, timer := range room.Timers[model.VotingState] {
						timer.Stop()
					}
					room.Timers[model.VotingState] = nil
					answers := make([]string, 0, len(room.Players)+1)
					if room.Truth != nil {
						answers = append(answers, *room.Truth)
					}
					for _, p := range room.Players {
						if p.Answer != nil {
							answers = append(answers, *p.Answer)
						}
					}

					mrand.Shuffle(len(answers), func(i, j int) {
						answers[i], answers[j] = answers[j], answers[i]
					})
					// Kept so a client connecting mid-vote sees the same list.
					room.Answers = answers
					room.PhaseEndsAt = time.Now().Add(60 * time.Second)
					s.broadcastRoom(room, map[string]any{
						"type": "voting_started",
						"payload": map[string]any{
							"answers": answers,
						},
					})
					timer := time.AfterFunc(60*time.Second, func() {
						room.StateTransition <- model.ResultState
					})
					room.Timers[model.ResultState] = append(
						room.Timers[model.ResultState],
						timer,
					)
					s.mu.Unlock()
				case model.ResultState:
					s.mu.Lock()
					if room.State != model.VotingState {
						s.mu.Unlock()
						continue
					}
					room.State = nextState
					for _, timer := range room.Timers[model.ResultState] {
						timer.Stop()
					}
					room.Timers[model.ResultState] = nil
					room.Answers = nil
					room.PhaseEndsAt = time.Now().Add(15 * time.Second)

					results := s.prepareResults(room)
					s.broadcastRoom(room, map[string]any{
						"type": "round_over",
						"payload": map[string]any{
							"truth":   room.Truth,
							"results": results,
						},
					})
					next := model.LobbyState
					if room.Round >= maxRounds {
						next = model.FinishedState
					}
					timer := time.AfterFunc(15*time.Second, func() {
						room.StateTransition <- next
					})
					room.Timers[next] = append(
						room.Timers[next], timer,
					)
					s.mu.Unlock()
				case model.FinishedState:
					s.mu.Lock()
					if room.State != model.ResultState {
						s.mu.Unlock()
						continue
					}
					room.State = model.FinishedState
					for _, timer := range room.Timers[model.FinishedState] {
						timer.Stop()
					}
					room.Timers[model.FinishedState] = nil
					results := s.prepareFinalResults(room)
					s.mu.Unlock()
					s.broadcastRoom(room, map[string]any{
						"type":    "game_over",
						"payload": map[string]any{"final_results": results},
					})
				}
			}
		}
	}()
	s.rooms[room.Code] = room
	s.userRooms[userID] = room
	s.scheduleClose(room)
	return room, nil
}

func (s *GameService) scheduleClose(room *model.Room) {
	time.AfterFunc(5*time.Minute, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		for _, p := range room.Players {
			if p.MessageBus != nil {
				s.scheduleClose(room) // still occupied, look again later
				return
			}
		}
		for _, timers := range room.Timers {
			for _, t := range timers {
				t.Stop()
			}
		}
		room.Timers = map[model.GameState][]*time.Timer{}

		delete(s.rooms, room.Code)
		delete(s.userRooms, room.HostID)
		close(room.Done) // never close Messages or StateTransition
	})
}

func (s *GameService) Join(code string, player *model.Player) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[code]
	if !ok {
		return ErrRoomNotExists
	}

	for _, p := range room.Players {
		if player.Nickname == p.Nickname {
			return ErrNicknameTaken
		}
	}

	token, err := randString(charset, 16)
	if err != nil {
		log.Printf("generate token err: %s\n", err.Error())
		return ErrTokenGenerate
	}

	player.Token = token
	player.JoinedRound = room.Round

	room.Players = append(room.Players, player)
	return nil
}

func (s *GameService) SendToPlayer(player *model.Player, payload any) {
	if player == nil || player.MessageBus == nil {
		return
	}
	message, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal message to %s err: %s\n", player.Nickname, err.Error())
		return
	}
	select {
	case player.MessageBus <- message:
	default:
		log.Printf("player %s inbox full, dropping message\n", player.Nickname)
	}
}

// roomSnapshot builds everything a freshly connected client needs to render the
// room without waiting for the next transition. Callers must hold s.mu.
//
// The viewer's own progress is included, but never the truth and never another
// player's answer before voting opens.
func (s *GameService) roomSnapshot(room *model.Room, viewer *model.Player) map[string]any {
	players := make([]model.PlayerState, 0, len(room.Players))
	for _, p := range room.Players {
		players = append(players, model.PlayerState{
			Nickname:  p.Nickname,
			Score:     p.Score,
			ScoreDiff: p.ScoreDiff,
			IsReady:   p.IsReady,
			Connected: p.MessageBus != nil,
		})
	}

	payload := map[string]any{
		"code":       room.Code,
		"state":      room.State,
		"round":      room.Round,
		"max_rounds": maxRounds,
		"nickname":   viewer.Nickname,
		"is_ready":   viewer.IsReady,
		"answered":   viewer.Answer != nil,
		"voted":      viewer.Vote != nil,
		"players":    players,
		"messages":   room.MessageLog,
	}
	if viewer.Answer != nil {
		payload["answer"] = *viewer.Answer
	}
	if viewer.Vote != nil {
		payload["vote"] = *viewer.Vote
	}
	// The situation is public from the moment the round starts.
	if room.Prompt != nil && (room.State == model.LieState || room.State == model.VotingState) {
		payload["prompt"] = *room.Prompt
	}
	// Candidates are public only once voting opens.
	if room.State == model.VotingState && room.Answers != nil {
		payload["answers"] = room.Answers
	}
	if room.State == model.ResultState {
		payload["truth"] = room.Truth
		payload["results"] = s.prepareResults(room)
	}
	if room.State == model.FinishedState {
		payload["final_results"] = s.prepareFinalResults(room)
	}
	if remaining := time.Until(room.PhaseEndsAt); !room.PhaseEndsAt.IsZero() && remaining > 0 {
		payload["phase_ends_in_ms"] = remaining.Milliseconds()
	}

	return map[string]any{"type": "room_state", "payload": payload}
}

func (s *GameService) broadcastRoom(room *model.Room, payload any) {
	message, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal message err: %s\n", err.Error())
		return
	}
	select {
	case room.Messages <- message:
	default:
		log.Printf("room %s outbound queue full, dropping message\n", room.Code)
	}
}

func (s *GameService) Dispatch(code, nickname string, action dtos.Action) error {
	room, next, err := s.dispatchLocked(code, nickname, action)
	if err != nil {
		return err
	}
	if next != nil {
		select {
		case room.StateTransition <- *next:
		case <-room.Done:
		}
	}
	return nil
}

func (s *GameService) dispatchLocked(code, nickname string, action dtos.Action) (*model.Room, *model.GameState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[code]
	if !ok {
		return nil, nil, ErrRoomNotExists
	}
	var player *model.Player
	for _, p := range room.Players {
		if p.Nickname == nickname {
			player = p
			break
		}
	}
	if player == nil {
		return nil, nil, ErrPlayerNotExists
	}
	dispatcher, ok := s.dispatchers[action.Type]
	if !ok {
		return room, nil, nil
	}
	next, err := dispatcher(s, room, player, action.Payload)
	return room, next, err
}

func (s *GameService) Connect(code string, token string, bus chan<- []byte) (*model.Player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[code]
	if !ok {
		return nil, ErrRoomNotExists
	}
	var player *model.Player
	for _, p := range room.Players {
		if token == p.Token {
			player = p
			break
		}
	}
	if player == nil {
		return nil, errors.New("token invalid")
	}
	if player.MessageBus != nil {
		return nil, errors.New("you are already connected")
	}
	player.MessageBus = bus
	// Sent before the broadcast so the client can render the room before it
	// starts receiving incremental updates.
	s.SendToPlayer(player, s.roomSnapshot(room, player))
	s.broadcastRoom(room, map[string]any{
		"type": "player_connected",
		"payload": model.PlayerState{
			Nickname:  player.Nickname,
			Score:     player.Score,
			IsReady:   player.IsReady,
			Connected: player.MessageBus != nil,
		},
	})

	return player, nil
}

func (s *GameService) Leave(code string, token string) {
	s.mu.Lock()
	room, ok := s.rooms[code]
	if !ok {
		s.mu.Unlock()
		return
	}
	var player *model.Player
	for _, p := range room.Players {
		if token == p.Token {
			p.MessageBus = nil
			p.IsReady = false
			p.LeavedRound = room.Round
			player = p
			break
		}
	}
	s.mu.Unlock()
	if player == nil {
		return
	}
	s.broadcastRoom(room, map[string]any{
		"type":    "player_disconnected",
		"payload": map[string]any{"nickname": player.Nickname},
	})

}

func (s *GameService) generateCode() (string, error) {
	var code string
	for i := 0; i < 3; i++ {
		c, err := randString(codeCharset, 4)
		if err != nil {
			return "", err
		}
		if _, ok := s.rooms[c]; !ok {
			code = c
			break
		}
	}
	if code == "" {
		return "", ErrRoomCodeGenerate
	}
	return code, nil
}

func randString(charset string, length int) (string, error) {
	maxByte := 255 - (256 % len(charset)) // 247 for a 62-char set
	out := make([]byte, 0, length)
	buf := make([]byte, length)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) > maxByte {
				continue
			}
			out = append(out, charset[int(b)%len(charset)])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}

func ErrFrame(message string) map[string]any {
	return map[string]any{
		"type":    "error",
		"payload": map[string]any{"message": message},
	}
}

func dispatchMessage(s *GameService, room *model.Room, player *model.Player, payload *string) (*model.GameState, error) {
	if payload == nil {
		s.SendToPlayer(player, ErrFrame("message must be inside payload field"))
		return nil, nil
	}
	text := strings.TrimSpace(*payload)
	if text == "" {
		s.SendToPlayer(player, ErrFrame("message must not be empty"))
		return nil, nil
	}
	if r := []rune(text); len(r) > maxMessageRunes {
		text = string(r[:maxMessageRunes])
	}
	msg := dtos.Message{From: player.Nickname, Message: text}
	addMessage(msg, room)
	s.broadcastRoom(room, map[string]any{
		"type":    "message_sent",
		"payload": msg,
	})
	return nil, nil
}

func dispatchReady(s *GameService, room *model.Room, player *model.Player, payload *string) (*model.GameState, error) {
	if room.State != model.LobbyState {
		s.SendToPlayer(player, ErrFrame("you can be not ready only within lobby state"))
		return nil, nil
	}
	if player.IsReady {
		s.SendToPlayer(player, ErrFrame("you are ready"))
		return nil, nil
	}
	player.IsReady = true
	s.broadcastRoom(room, map[string]any{
		"type": "player_ready",
		"payload": map[string]any{
			"nickname": player.Nickname,
		},
	})
	allReady := true
	playersReady := 0
	for _, p := range room.Players {
		if p.IsReady && p.MessageBus != nil {
			playersReady += 1
		}
		if !p.IsReady && p.MessageBus != nil {
			allReady = false
		}
	}
	if allReady && playersReady >= 2 {
		next := model.LieState
		return &next, nil
	}

	return nil, nil
}

func dispatchRestart(s *GameService, room *model.Room, player *model.Player, payload *string) (*model.GameState, error) {
	if room.State != model.FinishedState {
		s.SendToPlayer(player, ErrFrame("you can restart only finished game"))
		return nil, nil
	}
	room.Prompt = nil
	room.Truth = nil
	for _, p := range room.Players {
		p.Score = 0
	}
	room.Round = 1
	room.UsedPromptIDs = room.UsedPromptIDs[:0]
	next := model.LobbyState

	return &next, nil
}

func dispatchLie(s *GameService, room *model.Room, player *model.Player, payload *string) (*model.GameState, error) {
	if room.State != model.LieState {
		s.SendToPlayer(player, ErrFrame("promt must be sent only within prompt state"))
		return nil, nil
	}
	if payload == nil {
		s.SendToPlayer(player, ErrFrame("promt must be inside payload field"))
		return nil, nil
	}
	answer := strings.ToLower(strings.TrimSpace(*payload))
	if answer == "" {
		s.SendToPlayer(player, ErrFrame("prompt must not be empty"))
		return nil, nil
	}
	if player.Answer != nil {
		s.SendToPlayer(player, ErrFrame("you already answered"))
		return nil, nil
	}
	if room.Truth != nil && answer == *room.Truth {
		s.SendToPlayer(player, ErrFrame("that answer is already taken"))
		return nil, nil
	}
	for _, p := range room.Players {
		if p.MessageBus != nil && p.Answer != nil && *p.Answer == answer {
			s.SendToPlayer(player, ErrFrame("that answer is already taken"))
			return nil, nil
		}
	}
	player.Answer = &answer
	s.broadcastRoom(room, map[string]any{
		"type": "player_lied",
		"payload": map[string]any{
			"nickname": player.Nickname,
		},
	})
	allAnswered := true
	for _, p := range room.Players {
		if p.MessageBus != nil && p.Answer == nil {
			allAnswered = false
			break
		}
	}
	if allAnswered {
		next := model.VotingState
		return &next, nil
	}
	return nil, nil
}

func dispatchVote(s *GameService, room *model.Room, player *model.Player, payload *string) (*model.GameState, error) {
	if room.State != model.VotingState {
		s.SendToPlayer(player, ErrFrame("vote must be sent only within voting state"))
		return nil, nil
	}
	if payload == nil {
		s.SendToPlayer(player, ErrFrame("vote must be inside payload field"))
		return nil, nil
	}
	vote := strings.ToLower(strings.TrimSpace(*payload))
	if vote == "" {
		s.SendToPlayer(player, ErrFrame("vote must not be empty"))
		return nil, nil
	}
	if player.Vote != nil {
		s.SendToPlayer(player, ErrFrame("you already voted"))
		return nil, nil
	}
	if player.Answer != nil && *player.Answer == vote {
		s.SendToPlayer(player, ErrFrame("you cant vote for own answer"))
		return nil, nil
	}
	player.Vote = &vote

	if room.Truth != nil && vote == *room.Truth {
		player.ScoreDiff += 1000
	} else {
		for _, p := range room.Players {
			if p != player && p.Answer != nil && *p.Answer == vote {
				p.ScoreDiff += 500
				break
			}
		}
	}
	s.broadcastRoom(room, map[string]any{
		"type": "player_voted",
		"payload": map[string]any{
			"nickname": player.Nickname,
		},
	})
	for _, p := range room.Players {
		if p.MessageBus != nil && p.Vote == nil {
			return nil, nil
		}
	}
	next := model.ResultState
	return &next, nil
}

func (s *GameService) prepareResults(room *model.Room) []model.PlayerResult {
	if room == nil {
		return make([]model.PlayerResult, 0)
	}
	scores := make([]model.PlayerResult, 0, len(room.Players))
	for _, player := range room.Players {
		scores = append(
			scores,
			model.PlayerResult{
				Nickname:  player.Nickname,
				Score:     player.Score,
				ScoreDiff: player.ScoreDiff,
				Answer:    player.Answer,
				Vote:      player.Vote,
			},
		)
	}
	slices.SortFunc(scores, func(a, b model.PlayerResult) int {
		return cmp.Compare(b.Score+b.ScoreDiff, a.Score+b.ScoreDiff)
	})
	return scores
}

func (s *GameService) prepareFinalResults(room *model.Room) []model.FinalResult {
	if room == nil {
		return make([]model.FinalResult, 0)
	}
	scores := make([]model.FinalResult, 0, len(room.Players))
	for _, player := range room.Players {
		scores = append(
			scores,
			model.FinalResult{
				Nickname: player.Nickname,
				Score:    player.Score,
			},
		)
	}
	slices.SortFunc(scores, func(a, b model.FinalResult) int {
		return cmp.Compare(b.Score, a.Score)
	})
	if len(scores) == 0 {
		return scores
	}
	position := 1
	prevScore := scores[0].Score
	for i := range scores {
		if prevScore > scores[i].Score {
			position++
		}
		scores[i].Position = uint(position)
	}
	return scores
}

func (s *GameService) resetPlayers(room *model.Room) {
	if room == nil {
		return
	}
	for _, player := range room.Players {
		player.IsReady = false
		player.Score += player.ScoreDiff
		player.ScoreDiff = 0
		player.Answer = nil
		player.Vote = nil
	}
}

func addMessage(message dtos.Message, room *model.Room) {
	room.MessageLog = append(room.MessageLog, message)
	if len(room.MessageLog) > 100 {
		room.MessageLog = room.MessageLog[len(room.MessageLog)-100:]
	}
}
