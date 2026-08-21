package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cheyzie/pav_game/internal/dtos"
	"github.com/Cheyzie/pav_game/internal/model"
)

// ---------------------------------------------------------------------------
// stubs
// ---------------------------------------------------------------------------

type stubGameRepo struct{}

func (stubGameRepo) Store(*model.Game) error { return nil }

type stubPromptRepo struct {
	mu      sync.Mutex
	prompts []model.Prompt
	err     error
	calls   int
}

func (r *stubPromptRepo) Store(*model.Prompt) error { return nil }

func (r *stubPromptRepo) GetRand(used []uint) (model.Prompt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return model.Prompt{}, r.err
	}
	for _, p := range r.prompts {
		if !slices.Contains(used, p.ID) {
			return p, nil
		}
	}
	return model.Prompt{}, errors.New("no prompts left")
}

func (r *stubPromptRepo) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *stubPromptRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// ---------------------------------------------------------------------------
// frames
// ---------------------------------------------------------------------------

// frame is the envelope every server message uses: {"type", "payload"}.
type frame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (f frame) name() string { return f.Type }

// payloadHas reports whether the raw payload mentions text. Error payloads are
// not uniformly shaped, so match on the encoded bytes.
func (f frame) payloadHas(text string) bool {
	return bytes.Contains(f.Payload, []byte(text))
}

type nicknamePayload struct {
	Nickname string `json:"nickname"`
}

type roomStatePayload struct {
	Code          string              `json:"code"`
	State         model.GameState     `json:"state"`
	Round         uint                `json:"round"`
	MaxRounds     uint                `json:"max_rounds"`
	Nickname      string              `json:"nickname"`
	IsReady       bool                `json:"is_ready"`
	Answered      bool                `json:"answered"`
	Voted         bool                `json:"voted"`
	Answer        *string             `json:"answer"`
	Vote          *string             `json:"vote"`
	Prompt        *string             `json:"prompt"`
	Answers       []string            `json:"answers"`
	PhaseEndsInMS *int64              `json:"phase_ends_in_ms"`
	Players       []model.PlayerState `json:"players"`
}

// snapshot connects the client and returns the room_state frame it is sent.
func (c *testClient) snapshot() roomStatePayload {
	c.t.Helper()
	c.connect()
	return decodePayload[roomStatePayload](c.t, c.await("room_state"))
}

func playerState(states []model.PlayerState, nickname string) (model.PlayerState, bool) {
	for _, p := range states {
		if p.Nickname == nickname {
			return p, true
		}
	}
	return model.PlayerState{}, false
}

// ---------------------------------------------------------------------------
// harness
// ---------------------------------------------------------------------------

const (
	busSize      = 64
	awaitTimeout = 2 * time.Second
	quietWindow  = 250 * time.Millisecond
	hostID       = uint(1)
	// firstUserID keeps player account ids clear of hostID.
	firstUserID = uint(100)
)

var defaultPrompt = model.Prompt{
	ID:        1,
	Situation: "why is the sky blue?",
	Truth:     "Rayleigh Scattering", // deliberately mixed case
}

type harness struct {
	t       *testing.T
	svc     *GameService
	prompts *stubPromptRepo
	room    *model.Room
	// nextUserID hands every player a distinct account id. Join treats a
	// matching UserId as the same person returning, so sharing one id (the
	// zero value in particular) would fold the whole room into one player.
	nextUserID uint
}

func newHarness(t *testing.T, prompts ...model.Prompt) *harness {
	t.Helper()
	if len(prompts) == 0 {
		prompts = []model.Prompt{defaultPrompt}
	}
	repo := &stubPromptRepo{prompts: prompts}
	svc := NewGameService(stubGameRepo{}, repo)
	room, err := svc.CreateRoom(hostID)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	return &harness{t: t, svc: svc, prompts: repo, room: room, nextUserID: firstUserID}
}

type testClient struct {
	t        *testing.T
	svc      *GameService
	code     string
	nickname string
	token    string
	userID   uint
	bus      chan []byte
}

// join registers a player and opens a websocket-equivalent bus for it.
func (h *harness) join(nickname string) *testClient {
	h.t.Helper()
	c := h.joinOnly(nickname)
	c.connect()
	return c
}

// joinOnly registers a player over "HTTP" without ever opening a bus, which is
// how a player that never connects is represented.
func (h *harness) joinOnly(nickname string) *testClient {
	h.t.Helper()
	h.nextUserID++
	userID := h.nextUserID
	joined, err := h.svc.Join(h.room.Code, &model.Player{Nickname: nickname, UserId: userID})
	if err != nil {
		h.t.Fatalf("Join(%s): %v", nickname, err)
	}
	return &testClient{
		t:        h.t,
		svc:      h.svc,
		code:     h.room.Code,
		nickname: nickname,
		token:    joined.Token,
		userID:   userID,
	}
}

func (c *testClient) connect() {
	c.t.Helper()
	bus := make(chan []byte, busSize)
	if _, err := c.svc.Connect(c.code, c.token, bus); err != nil {
		c.t.Fatalf("Connect(%s): %v", c.nickname, err)
	}
	c.bus = bus
}

func (c *testClient) send(action dtos.ActionType, payload string) error {
	c.t.Helper()
	return c.svc.Dispatch(c.code, c.nickname, dtos.Action{Type: action, Payload: &payload})
}

func (c *testClient) sendNil(action dtos.ActionType) error {
	c.t.Helper()
	return c.svc.Dispatch(c.code, c.nickname, dtos.Action{Type: action})
}

// sendWithin fails if the dispatch does not return in time. Dispatch takes the
// service mutex, so a hang here means something leaked it — which would
// otherwise show up as the whole test binary timing out.
func (c *testClient) sendWithin(action dtos.ActionType, payload string, window time.Duration) {
	c.t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- c.svc.Dispatch(c.code, c.nickname, dtos.Action{Type: action, Payload: &payload})
	}()
	select {
	case err := <-done:
		if err != nil {
			c.t.Fatalf("%s: dispatch %q: %v", c.nickname, action, err)
		}
	case <-time.After(window):
		c.t.Fatalf("%s: dispatch %q blocked for %s — the service mutex is held",
			c.nickname, action, window)
	}
}

// await blocks until a frame with the given name arrives, skipping others.
func (c *testClient) await(name string) frame {
	c.t.Helper()
	deadline := time.After(awaitTimeout)
	for {
		select {
		case raw := <-c.bus:
			var f frame
			if err := json.Unmarshal(raw, &f); err != nil {
				c.t.Fatalf("%s: undecodable frame %q: %v", c.nickname, raw, err)
			}
			if f.name() == name {
				return f
			}
		case <-deadline:
			c.t.Fatalf("%s: timed out waiting for %q", c.nickname, name)
		}
	}
}

// awaitNone fails if a frame with the given name shows up inside the window.
func (c *testClient) awaitNone(name string, window time.Duration) {
	c.t.Helper()
	deadline := time.After(window)
	for {
		select {
		case raw := <-c.bus:
			var f frame
			if err := json.Unmarshal(raw, &f); err != nil {
				continue
			}
			if f.name() == name {
				c.t.Fatalf("%s: unexpected %q frame", c.nickname, name)
			}
		case <-deadline:
			return
		}
	}
}

// readyUp walks the room from lobby to the prompt phase.
func (h *harness) readyUp(clients ...*testClient) {
	h.t.Helper()
	for _, c := range clients {
		if err := c.send(dtos.ReadyAction, ""); err != nil {
			h.t.Fatalf("ready(%s): %v", c.nickname, err)
		}
	}
	for _, c := range clients {
		c.await("game_started")
	}
}

// answer submits one answer per client and waits for the voting phase.
func (h *harness) answerAll(answers map[*testClient]string) {
	h.t.Helper()
	for c, a := range answers {
		if err := c.send(dtos.LieAction, a); err != nil {
			h.t.Fatalf("prompt(%s): %v", c.nickname, err)
		}
	}
	for c := range answers {
		c.await("voting_started")
	}
}

type votingPayload struct {
	Answers []string `json:"answers"`
}

type resultsPayload struct {
	Results []model.PlayerResult `json:"results"`
}

// game_over reports the settled standings, which are a different shape from a
// round's results: a total and a placing, with no per-round delta.
type finalResultsPayload struct {
	FinalResults []model.FinalResult `json:"final_results"`
}

func decodePayload[T any](t *testing.T, f frame) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(f.Payload, &out); err != nil {
		t.Fatalf("decode %s payload %q: %v", f.name(), f.Payload, err)
	}
	return out
}

// scoreOf returns a player's score as of the end of the round. round_over
// reports Score as the carried-in total and ScoreDiff as this round's delta, so
// the client can animate the change; the settled total is the sum.
func scoreOf(scores []model.PlayerResult, nickname string) (uint, bool) {
	for _, s := range scores {
		if s.Nickname == nickname {
			return s.Score + s.ScoreDiff, true
		}
	}
	return 0, false
}

// finalResultOf returns a player's row from the game_over standings.
func finalResultOf(results []model.FinalResult, nickname string) (model.FinalResult, bool) {
	for _, r := range results {
		if r.Nickname == nickname {
			return r, true
		}
	}
	return model.FinalResult{}, false
}

// resultOf returns the whole result row, for assertions that care about the
// split between carried total and this round's delta.
func resultOf(scores []model.PlayerResult, nickname string) (model.PlayerResult, bool) {
	for _, s := range scores {
		if s.Nickname == nickname {
			return s, true
		}
	}
	return model.PlayerResult{}, false
}

// ---------------------------------------------------------------------------
// room and player management
// ---------------------------------------------------------------------------

func TestCreateRoomIsIdempotentPerHost(t *testing.T) {
	t.Parallel()
	svc := NewGameService(stubGameRepo{}, &stubPromptRepo{prompts: []model.Prompt{defaultPrompt}})

	first, err := svc.CreateRoom(7)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	again, err := svc.CreateRoom(7)
	if err != nil {
		t.Fatalf("CreateRoom (repeat): %v", err)
	}
	if first != again {
		t.Errorf("same host got two rooms: %q then %q", first.Code, again.Code)
	}

	other, err := svc.CreateRoom(8)
	if err != nil {
		t.Fatalf("CreateRoom (other host): %v", err)
	}
	if other.Code == first.Code {
		t.Errorf("different hosts share room code %q", other.Code)
	}
	if len(first.Code) != 4 {
		t.Errorf("room code %q: want 4 characters, got %d", first.Code, len(first.Code))
	}
	if first.State != model.LobbyState {
		t.Errorf("new room state = %q, want %q", first.State, model.LobbyState)
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	t.Run("unknown room", func(t *testing.T) {
		_, err := h.svc.Join("nope", &model.Player{Nickname: "ghost"})
		if !errors.Is(err, ErrRoomNotExists) {
			t.Errorf("got %v, want %v", err, ErrRoomNotExists)
		}
	})

	t.Run("issues a token", func(t *testing.T) {
		p, err := h.svc.Join(h.room.Code, &model.Player{Nickname: "alice", UserId: firstUserID})
		if err != nil {
			t.Fatalf("Join: %v", err)
		}
		if len(p.Token) != 16 {
			t.Errorf("token %q: want 16 characters, got %d", p.Token, len(p.Token))
		}
		if p.JoinedRound != 1 {
			t.Errorf("JoinedRound = %d, want 1", p.JoinedRound)
		}
	})

	t.Run("another account cannot take the nickname, even before connecting", func(t *testing.T) {
		_, err := h.svc.Join(h.room.Code, &model.Player{Nickname: "alice", UserId: firstUserID + 1})
		if !errors.Is(err, ErrNicknameTaken) {
			t.Errorf("got %v, want %v", err, ErrNicknameTaken)
		}
	})
}

// Join keys off the account id, not the nickname, so the same user arriving
// twice keeps one seat and one token rather than being issued a second player.
func TestJoinIsIdempotentPerAccount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	h.join("bob")
	carol := h.joinOnly("carol") // connects at the end to read the roster

	again, err := h.svc.Join(h.room.Code, &model.Player{Nickname: "alice", UserId: alice.userID})
	if err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	if again.Token != alice.token {
		t.Errorf("token = %q, want the original %q", again.Token, alice.token)
	}

	snap := carol.snapshot()
	if len(snap.Players) != 3 {
		t.Errorf("players = %+v, want 3 — the rejoin must not claim a second seat", snap.Players)
	}
}

// A returning user may come back under a new nickname. The seat and token
// follow them, and the room is told about the change.
func TestJoinRenamesReturningUser(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	again, err := h.svc.Join(h.room.Code, &model.Player{Nickname: "alicia", UserId: alice.userID})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if again.Nickname != "alicia" {
		t.Errorf("nickname = %q, want %q", again.Nickname, "alicia")
	}
	if again.Token != alice.token {
		t.Errorf("token = %q, want the original %q", again.Token, alice.token)
	}

	f := bob.await("player_renamed")
	if !f.payloadHas("alice") || !f.payloadHas("alicia") {
		t.Errorf("payload %q should carry both the old and the new name", f.Payload)
	}

	// The room answers to the new nickname...
	alice.nickname = "alicia"
	if err := alice.send(dtos.SendMessageAction, "still me"); err != nil {
		t.Fatalf("dispatch under the new nickname: %v", err)
	}
	bob.await("message_sent")

	// ...and no longer to the old one.
	err = h.svc.Dispatch(h.room.Code, "alice", dtos.Action{Type: dtos.ReadyAction})
	if !errors.Is(err, ErrPlayerNotExists) {
		t.Errorf("dispatch under the old nickname: got %v, want %v", err, ErrPlayerNotExists)
	}
}

// Renaming into someone else's nickname is still a clash.
func TestJoinRenameCannotStealANickname(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	h.join("bob")

	_, err := h.svc.Join(h.room.Code, &model.Player{Nickname: "bob", UserId: alice.userID})
	if !errors.Is(err, ErrNicknameTaken) {
		t.Errorf("got %v, want %v", err, ErrNicknameTaken)
	}
}

func TestConnect(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.joinOnly("alice")

	t.Run("bad token", func(t *testing.T) {
		if _, err := h.svc.Connect(h.room.Code, "not-a-token", make(chan []byte, busSize)); err == nil {
			t.Error("expected an error for an invalid token")
		}
	})

	t.Run("unknown room", func(t *testing.T) {
		_, err := h.svc.Connect("nope", alice.token, make(chan []byte, busSize))
		if !errors.Is(err, ErrRoomNotExists) {
			t.Errorf("got %v, want %v", err, ErrRoomNotExists)
		}
	})

	t.Run("succeeds and announces", func(t *testing.T) {
		alice.connect()
		f := alice.await("player_connected")
		if got := decodePayload[nicknamePayload](t, f).Nickname; got != "alice" {
			t.Errorf("nickname = %q, want %q", got, "alice")
		}
	})

	t.Run("second socket is refused", func(t *testing.T) {
		if _, err := h.svc.Connect(h.room.Code, alice.token, make(chan []byte, busSize)); err == nil {
			t.Error("expected an error connecting twice with one token")
		}
	})
}

func TestLeaveAllowsReconnect(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	h.svc.Leave(h.room.Code, alice.token)
	f := bob.await("player_disconnected")
	if got := decodePayload[nicknamePayload](t, f).Nickname; got != "alice" {
		t.Errorf("nickname = %q, want %q", got, "alice")
	}

	alice.connect() // same token, fresh bus
	bob.await("player_connected")
}

func TestDispatchUnknownRoomAndPlayer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.join("alice")

	err := h.svc.Dispatch("nope", "alice", dtos.Action{Type: dtos.ReadyAction})
	if !errors.Is(err, ErrRoomNotExists) {
		t.Errorf("unknown room: got %v, want %v", err, ErrRoomNotExists)
	}

	err = h.svc.Dispatch(h.room.Code, "mallory", dtos.Action{Type: dtos.ReadyAction})
	if !errors.Is(err, ErrPlayerNotExists) {
		t.Errorf("unknown player: got %v, want %v", err, ErrPlayerNotExists)
	}

	if err := h.svc.Dispatch(h.room.Code, "alice", dtos.Action{Type: "not_an_action"}); err != nil {
		t.Errorf("unknown action should be ignored, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// room_state snapshot
// ---------------------------------------------------------------------------

func TestSnapshotInLobby(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.joinOnly("bob") // connects below, so it sees itself connected
	h.joinOnly("dave")       // registered over HTTP, never opens a socket

	if err := alice.send(dtos.ReadyAction, ""); err != nil {
		t.Fatalf("ready(alice): %v", err)
	}
	alice.await("player_ready")

	snap := bob.snapshot()

	if snap.Code != h.room.Code {
		t.Errorf("code = %q, want %q", snap.Code, h.room.Code)
	}
	if snap.State != model.LobbyState {
		t.Errorf("state = %q, want %q", snap.State, model.LobbyState)
	}
	if snap.Round != 1 {
		t.Errorf("round = %d, want 1", snap.Round)
	}
	if snap.MaxRounds != maxRounds {
		t.Errorf("max_rounds = %d, want %d", snap.MaxRounds, maxRounds)
	}
	if snap.Nickname != "bob" {
		t.Errorf("nickname = %q, want %q", snap.Nickname, "bob")
	}
	if snap.IsReady || snap.Answered || snap.Voted {
		t.Errorf("bob should have no progress yet: %+v", snap)
	}
	if snap.Prompt != nil {
		t.Errorf("prompt leaked in the lobby: %q", *snap.Prompt)
	}
	if snap.Answers != nil {
		t.Errorf("answers leaked in the lobby: %v", snap.Answers)
	}
	if snap.PhaseEndsInMS != nil {
		t.Errorf("lobby is not on a clock, got phase_ends_in_ms = %d", *snap.PhaseEndsInMS)
	}

	// The roster reflects who is ready and who is actually connected.
	if len(snap.Players) != 3 {
		t.Fatalf("players = %+v, want 3 entries", snap.Players)
	}
	a, ok := playerState(snap.Players, "alice")
	if !ok {
		t.Fatalf("players %+v missing alice", snap.Players)
	}
	if !a.IsReady || !a.Connected {
		t.Errorf("alice = %+v, want ready and connected", a)
	}
	b, _ := playerState(snap.Players, "bob")
	if b.IsReady || !b.Connected {
		t.Errorf("bob = %+v, want not-ready and connected (he is the viewer)", b)
	}
	d, ok := playerState(snap.Players, "dave")
	if !ok {
		t.Fatalf("players %+v missing dave", snap.Players)
	}
	if d.Connected {
		t.Errorf("dave = %+v, want connected=false (never opened a socket)", d)
	}
}

func TestSnapshotDuringPromptPhase(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")
	carol := h.joinOnly("carol")

	h.readyUp(alice, bob)
	if err := alice.send(dtos.LieAction, "a lizard"); err != nil {
		t.Fatalf("prompt(alice): %v", err)
	}

	snap := carol.snapshot()

	if snap.State != model.LieState {
		t.Errorf("state = %q, want %q", snap.State, model.LieState)
	}
	if snap.Prompt == nil || *snap.Prompt != defaultPrompt.Situation {
		t.Errorf("prompt = %v, want %q", snap.Prompt, defaultPrompt.Situation)
	}
	// Alice has answered but nothing is public until voting opens.
	if snap.Answers != nil {
		t.Errorf("answers leaked during the prompt phase: %v", snap.Answers)
	}
	if snap.PhaseEndsInMS == nil {
		t.Fatal("prompt phase is on a clock, want phase_ends_in_ms")
	}
	if *snap.PhaseEndsInMS <= 0 || *snap.PhaseEndsInMS > 60_000 {
		t.Errorf("phase_ends_in_ms = %d, want within (0, 60000]", *snap.PhaseEndsInMS)
	}
}

func TestSnapshotReflectsOwnProgress(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})
	if err := alice.send(dtos.VoteAction, "solar wind"); err != nil {
		t.Fatalf("vote(alice): %v", err)
	}

	// Alice drops and comes back mid-vote.
	h.svc.Leave(h.room.Code, alice.token)
	bob.await("player_disconnected")
	snap := alice.snapshot()

	if snap.State != model.VotingState {
		t.Errorf("state = %q, want %q", snap.State, model.VotingState)
	}
	if !snap.Answered || snap.Answer == nil || *snap.Answer != "a lizard" {
		t.Errorf("answered=%v answer=%v, want her own answer echoed back", snap.Answered, snap.Answer)
	}
	if !snap.Voted || snap.Vote == nil || *snap.Vote != "solar wind" {
		t.Errorf("voted=%v vote=%v, want her own vote echoed back", snap.Voted, snap.Vote)
	}

	// The candidate list is the same one the room was shown.
	if len(snap.Answers) != 3 {
		t.Fatalf("answers = %v, want 3 entries", snap.Answers)
	}
	for _, want := range []string{"rayleigh scattering", "a lizard", "solar wind"} {
		if !slices.Contains(snap.Answers, want) {
			t.Errorf("answers %v missing %q", snap.Answers, want)
		}
	}
	if snap.Prompt == nil {
		t.Error("prompt should still be visible while voting")
	}
}

// The snapshot must never hand the client the truth as a distinguishable field.
func TestSnapshotDoesNotLeakTheTruth(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")
	carol := h.joinOnly("carol")

	h.readyUp(alice, bob)

	carol.connect()
	raw := carol.await("room_state")
	if raw.payloadHas("truth") {
		t.Errorf("snapshot mentions a truth field: %s", raw.Payload)
	}
	if raw.payloadHas(strings.ToLower(defaultPrompt.Truth)) {
		t.Errorf("snapshot leaks the answer during the prompt phase: %s", raw.Payload)
	}
}

// A reconnecting player is caught up on scores carried over from earlier rounds.
func TestSnapshotCarriesScores(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})
	if err := alice.send(dtos.VoteAction, "rayleigh scattering"); err != nil {
		t.Fatalf("vote(alice): %v", err)
	}
	if err := bob.send(dtos.VoteAction, "a lizard"); err != nil {
		t.Fatalf("vote(bob): %v", err)
	}
	alice.await("round_over")

	// Stand in for the 15s result timer. Entering the lobby is what folds the
	// round's delta into the carried total and opens the next round.
	h.room.StateTransition <- model.LobbyState
	alice.await("game_waiting")

	h.svc.Leave(h.room.Code, alice.token)
	bob.await("player_disconnected")
	snap := alice.snapshot()

	a, ok := playerState(snap.Players, "alice")
	if !ok {
		t.Fatalf("players %+v missing alice", snap.Players)
	}
	if a.Score != 1500 {
		t.Errorf("alice score = %d, want 1500", a.Score)
	}
	if snap.Round != 2 {
		t.Errorf("round = %d, want 2 after the first round closed", snap.Round)
	}
	// Round state was reset, so she is not still holding last round's answer.
	if snap.Answered || snap.Voted {
		t.Errorf("answered=%v voted=%v, want both cleared for the new round", snap.Answered, snap.Voted)
	}
}

// ---------------------------------------------------------------------------
// the round
// ---------------------------------------------------------------------------

func TestFullRoundLifecycle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	// A single ready is not enough to start.
	if err := alice.send(dtos.ReadyAction, ""); err != nil {
		t.Fatalf("ready(alice): %v", err)
	}
	bob.await("player_ready")
	bob.awaitNone("game_started", quietWindow)

	if err := bob.send(dtos.ReadyAction, ""); err != nil {
		t.Fatalf("ready(bob): %v", err)
	}
	started := alice.await("game_started")
	bob.await("game_started")
	if !started.payloadHas(defaultPrompt.Situation) {
		t.Errorf("game_started payload %q does not carry the situation", started.Payload)
	}

	// One answer alone does not advance the phase.
	if err := alice.send(dtos.LieAction, "a lizard"); err != nil {
		t.Fatalf("prompt(alice): %v", err)
	}
	alice.awaitNone("voting_started", quietWindow)

	if err := bob.send(dtos.LieAction, "solar wind"); err != nil {
		t.Fatalf("prompt(bob): %v", err)
	}
	voting := alice.await("voting_started")
	bob.await("voting_started")

	answers := decodePayload[votingPayload](t, voting).Answers
	if len(answers) != 3 {
		t.Fatalf("answers = %v, want 3 entries (truth + 2 players)", answers)
	}
	for _, want := range []string{"rayleigh scattering", "a lizard", "solar wind"} {
		if !slices.Contains(answers, want) {
			t.Errorf("answers %v missing %q", answers, want)
		}
	}

	// Alice finds the truth (+1000); Bob falls for Alice's answer (+500 to Alice).
	if err := alice.send(dtos.VoteAction, "Rayleigh Scattering"); err != nil {
		t.Fatalf("vote(alice): %v", err)
	}
	alice.awaitNone("round_over", quietWindow)

	if err := bob.send(dtos.VoteAction, "A Lizard"); err != nil {
		t.Fatalf("vote(bob): %v", err)
	}
	over := alice.await("round_over")

	results := decodePayload[resultsPayload](t, over).Results
	aliceScore, ok := scoreOf(results, "alice")
	if !ok {
		t.Fatalf("results %v missing alice", results)
	}
	if aliceScore != 1500 {
		t.Errorf("alice score = %d, want 1500 (1000 truth + 500 fooled bob)", aliceScore)
	}
	// round_over splits the carried total from this round's delta.
	aliceResult, _ := resultOf(results, "alice")
	if aliceResult.Score != 0 || aliceResult.ScoreDiff != 1500 {
		t.Errorf("alice = {score:%d diff:%d}, want {0, 1500} on the first round",
			aliceResult.Score, aliceResult.ScoreDiff)
	}
	if aliceResult.Answer == nil || *aliceResult.Answer != "a lizard" {
		t.Errorf("alice answer = %v, want it echoed in the results", aliceResult.Answer)
	}
	if aliceResult.Vote == nil || *aliceResult.Vote != "rayleigh scattering" {
		t.Errorf("alice vote = %v, want it echoed in the results", aliceResult.Vote)
	}
	bobScore, ok := scoreOf(results, "bob")
	if !ok {
		t.Fatalf("results %v missing bob", results)
	}
	if bobScore != 0 {
		t.Errorf("bob score = %d, want 0", bobScore)
	}
}

func TestDisconnectedPlayersDoNotBlockPhases(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	// Carol registers and connects, then drops before the game starts.
	carol := h.join("carol")
	h.svc.Leave(h.room.Code, carol.token)
	alice.await("player_disconnected")

	// Dave never opens a bus at all.
	h.joinOnly("dave")

	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})
}

// ---------------------------------------------------------------------------
// transition guards (regressions)
// ---------------------------------------------------------------------------

// A timer that fires after the phase already advanced must not run the
// transition a second time.
func TestDuplicateVotingTransitionIsIgnored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})

	// Stand in for a late 60s timer.
	h.room.StateTransition <- model.VotingState
	alice.awaitNone("voting_started", quietWindow)
}

// playRound drives one complete round, ending on round_over.
func (h *harness) playRound(alice, bob *testClient) []model.PlayerResult {
	h.t.Helper()
	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})

	if err := alice.send(dtos.VoteAction, "rayleigh scattering"); err != nil {
		h.t.Fatalf("vote(alice): %v", err)
	}
	if err := bob.send(dtos.VoteAction, "a lizard"); err != nil {
		h.t.Fatalf("vote(bob): %v", err)
	}
	return decodePayload[resultsPayload](h.t, alice.await("round_over")).Results
}

// After maxRounds the room must announce game_over and settle in the finished
// state, which is the only state a restart is accepted from.
// fullGamePrompts supplies one distinct prompt per round, since a room refuses
// to reuse a prompt until it is restarted.
func fullGamePrompts() []model.Prompt {
	prompts := make([]model.Prompt, 0, maxRounds)
	for i := range maxRounds {
		prompts = append(prompts, model.Prompt{
			ID:        uint(i + 1),
			Situation: defaultPrompt.Situation,
			Truth:     defaultPrompt.Truth,
		})
	}
	return prompts
}

// playToFinish runs a whole game and leaves the room in the finished state.
func (h *harness) playToFinish(alice, bob *testClient) []model.FinalResult {
	h.t.Helper()
	for round := 1; round <= maxRounds; round++ {
		h.playRound(alice, bob)
		if round < maxRounds {
			// Stand in for the 15s result timer rather than sleeping through it.
			// Entering the lobby cancels that pending timer, so it cannot fire late.
			h.room.StateTransition <- model.LobbyState
			alice.await("game_waiting")
		}
	}
	// Stand in for the 15s timer the result phase armed with FinishedState.
	h.room.StateTransition <- model.FinishedState
	return decodePayload[finalResultsPayload](h.t, alice.await("game_over")).FinalResults
}

func TestGameEndsAfterMaxRounds(t *testing.T) {
	t.Parallel()
	h := newHarness(t, fullGamePrompts()...)
	alice := h.join("alice")
	bob := h.join("bob")
	carol := h.joinOnly("carol") // connects at the end to read the settled state

	over := h.playToFinish(alice, bob)
	// Alice takes 1500 every round, so every round she played must be in the
	// settled total — including the last one.
	want := uint(maxRounds) * 1500
	aliceFinal, ok := finalResultOf(over, "alice")
	if !ok {
		t.Fatalf("standings %+v missing alice", over)
	}
	if aliceFinal.Score != want {
		t.Errorf("alice final score = %d, want %d (1500 for each of %d rounds)",
			aliceFinal.Score, want, maxRounds)
	}
	if aliceFinal.Position != 1 {
		t.Errorf("alice position = %d, want 1", aliceFinal.Position)
	}
	bobFinal, ok := finalResultOf(over, "bob")
	if !ok {
		t.Fatalf("standings %+v missing bob", over)
	}
	if bobFinal.Position != 2 {
		t.Errorf("bob position = %d, want 2", bobFinal.Position)
	}

	// The room must report itself finished, or a restart can never be accepted.
	snap := carol.snapshot()
	if snap.State != model.FinishedState {
		t.Errorf("state = %q, want %q after the final round", snap.State, model.FinishedState)
	}
}

func TestRestartRejectedBeforeFinish(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")

	if err := alice.sendNil(dtos.ResetAction); err != nil {
		t.Fatalf("reset: %v", err)
	}
	f := alice.await("error")
	if !f.payloadHas("finished") {
		t.Errorf("payload %q does not explain the restart was rejected", f.Payload)
	}
	alice.awaitNone("game_waiting", quietWindow)
}

// A restart must clear scores and the used-prompt set, and leave the room
// playable again from round one.
func TestRestartAfterFinish(t *testing.T) {
	t.Parallel()
	h := newHarness(t, fullGamePrompts()...)
	alice := h.join("alice")
	bob := h.join("bob")
	carol := h.joinOnly("carol")

	h.playToFinish(alice, bob)

	if err := alice.sendNil(dtos.ResetAction); err != nil {
		t.Fatalf("reset: %v", err)
	}
	alice.await("game_waiting")

	snap := carol.snapshot()
	if snap.State != model.LobbyState {
		t.Errorf("state = %q, want %q after a restart", snap.State, model.LobbyState)
	}
	if snap.Round != 1 {
		t.Errorf("round = %d, want 1 after a restart", snap.Round)
	}
	for _, p := range snap.Players {
		if p.Score != 0 || p.ScoreDiff != 0 {
			t.Errorf("%s = {score:%d diff:%d}, want both cleared after a restart",
				p.Nickname, p.Score, p.ScoreDiff)
		}
		if p.IsReady {
			t.Errorf("%s is still ready after a restart", p.Nickname)
		}
	}
	// Carol only ever observes; connected-but-not-ready would block the phase.
	h.svc.Leave(h.room.Code, carol.token)
	alice.await("player_disconnected")

	// The prompt pool was released, so a fresh game can be played out.
	results := h.playRound(alice, bob)
	if score, _ := scoreOf(results, "alice"); score != 1500 {
		t.Errorf("alice score = %d after the first round of game two, want 1500", score)
	}

	// A second restart is refused now that the room is mid-game again.
	if err := alice.sendNil(dtos.ResetAction); err != nil {
		t.Fatalf("reset: %v", err)
	}
	alice.await("error")
}

// A prompt transition that arrives when the room is no longer in the lobby must
// be dropped without wedging the state machine.
func TestStalePromptTransitionIsIgnored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	h.readyUp(alice, bob) // room is now in the prompt phase

	h.room.StateTransition <- model.LieState
	alice.awaitNone("game_started", quietWindow)

	// The service must still serve other rooms and actions afterwards.
	alice.sendWithin(dtos.SendMessageAction, "still alive", time.Second)
	bob.await("message_sent")
}

func TestDuplicateResultTransitionIsIgnored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})

	if err := alice.send(dtos.VoteAction, "rayleigh scattering"); err != nil {
		t.Fatalf("vote(alice): %v", err)
	}
	if err := bob.send(dtos.VoteAction, "a lizard"); err != nil {
		t.Fatalf("vote(bob): %v", err)
	}
	alice.await("round_over")

	h.room.StateTransition <- model.ResultState
	alice.awaitNone("round_over", quietWindow)
}

// The state machine must survive a transition it cannot service.
func TestStateMachineSurvivesStaleTransitions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	// Both are illegal from the lobby and must be dropped, not fatal.
	h.room.StateTransition <- model.VotingState
	h.room.StateTransition <- model.ResultState

	h.readyUp(alice, bob)
}

// ---------------------------------------------------------------------------
// rejections
// ---------------------------------------------------------------------------

func TestPromptRejections(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	// Wrong phase: still in the lobby.
	if err := alice.send(dtos.LieAction, "too early"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	alice.await("error")

	h.readyUp(alice, bob)

	t.Run("empty", func(t *testing.T) {
		if err := alice.send(dtos.LieAction, "   "); err != nil {
			t.Fatalf("prompt: %v", err)
		}
		alice.await("error")
	})

	t.Run("nil payload", func(t *testing.T) {
		if err := alice.sendNil(dtos.LieAction); err != nil {
			t.Fatalf("prompt: %v", err)
		}
		alice.await("error")
	})

	t.Run("matches the truth", func(t *testing.T) {
		if err := alice.send(dtos.LieAction, "  Rayleigh SCATTERING "); err != nil {
			t.Fatalf("prompt: %v", err)
		}
		alice.await("error")
	})

	// A real answer lands.
	if err := alice.send(dtos.LieAction, "a lizard"); err != nil {
		t.Fatalf("prompt(alice): %v", err)
	}

	t.Run("answering twice", func(t *testing.T) {
		if err := alice.send(dtos.LieAction, "something else"); err != nil {
			t.Fatalf("prompt: %v", err)
		}
		alice.await("error")
	})

	t.Run("duplicate of another player", func(t *testing.T) {
		if err := bob.send(dtos.LieAction, "A LIZARD"); err != nil {
			t.Fatalf("prompt: %v", err)
		}
		bob.await("error")
		// The rejection must not have counted as Bob's answer.
		bob.awaitNone("voting_started", quietWindow)
	})

	// Bob answers for real and the phase advances, proving none of the
	// rejected submissions were recorded.
	if err := bob.send(dtos.LieAction, "solar wind"); err != nil {
		t.Fatalf("prompt(bob): %v", err)
	}
	voting := bob.await("voting_started")
	if got := len(decodePayload[votingPayload](t, voting).Answers); got != 3 {
		t.Errorf("answers count = %d, want 3", got)
	}
}

func TestVoteRejections(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	// Wrong phase: still in the lobby.
	if err := alice.send(dtos.VoteAction, "anything"); err != nil {
		t.Fatalf("vote: %v", err)
	}
	alice.await("error")

	h.readyUp(alice, bob)
	h.answerAll(map[*testClient]string{alice: "a lizard", bob: "solar wind"})

	t.Run("own answer", func(t *testing.T) {
		if err := alice.send(dtos.VoteAction, "a lizard"); err != nil {
			t.Fatalf("vote: %v", err)
		}
		f := alice.await("error")
		if !f.payloadHas("own answer") {
			t.Errorf("payload %q does not mention voting for one's own answer", f.Payload)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if err := alice.send(dtos.VoteAction, "  "); err != nil {
			t.Fatalf("vote: %v", err)
		}
		alice.await("error")
	})

	if err := alice.send(dtos.VoteAction, "solar wind"); err != nil {
		t.Fatalf("vote(alice): %v", err)
	}

	t.Run("voting twice", func(t *testing.T) {
		if err := alice.send(dtos.VoteAction, "rayleigh scattering"); err != nil {
			t.Fatalf("vote: %v", err)
		}
		alice.await("error")
	})

	// Only Bob is outstanding, so the round closes once he votes. Alice's
	// second vote must not have scored her the truth bonus.
	if err := bob.send(dtos.VoteAction, "rayleigh scattering"); err != nil {
		t.Fatalf("vote(bob): %v", err)
	}
	results := decodePayload[resultsPayload](t, bob.await("round_over")).Results
	if score, _ := scoreOf(results, "alice"); score != 0 {
		t.Errorf("alice score = %d, want 0 (her only valid vote was wrong)", score)
	}
	if score, _ := scoreOf(results, "bob"); score != 1500 {
		t.Errorf("bob score = %d, want 1500 (1000 truth + 500 fooling alice)", score)
	}
}

func TestChatIsBroadcast(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.join("alice")
	bob := h.join("bob")

	if err := alice.send(dtos.SendMessageAction, "hello room"); err != nil {
		t.Fatalf("message: %v", err)
	}
	f := bob.await("message_sent")
	if !f.payloadHas("hello room") || !f.payloadHas("alice") {
		t.Errorf("payload %q missing text or sender", f.Payload)
	}
}

// ---------------------------------------------------------------------------
// prompt repository failure
// ---------------------------------------------------------------------------

func TestPromptFetchFailureReturnsToLobby(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("waits on the 5s lobby recovery timer")
	}
	h := newHarness(t)
	h.prompts.setErr(errors.New("database is down"))

	alice := h.join("alice")
	bob := h.join("bob")

	if err := alice.send(dtos.ReadyAction, ""); err != nil {
		t.Fatalf("ready(alice): %v", err)
	}
	if err := bob.send(dtos.ReadyAction, ""); err != nil {
		t.Fatalf("ready(bob): %v", err)
	}

	alice.awaitNone("game_started", quietWindow)
	if h.prompts.callCount() == 0 {
		t.Fatal("GetRand was never called")
	}

	// The room recovers to the lobby rather than wedging.
	awaitWithin(t, alice, "game_waiting", 8*time.Second)

	// Readiness was cleared, so players can start a fresh attempt.
	h.prompts.setErr(nil)
	if err := alice.send(dtos.ReadyAction, ""); err != nil {
		t.Fatalf("ready(alice) after recovery: %v", err)
	}
	alice.await("player_ready")
}

// awaitWithin is await with a caller-supplied deadline, for the one case that
// has to outlast a server-side timer.
func awaitWithin(t *testing.T, c *testClient, name string, window time.Duration) frame {
	t.Helper()
	deadline := time.After(window)
	for {
		select {
		case raw := <-c.bus:
			var f frame
			if err := json.Unmarshal(raw, &f); err != nil {
				continue
			}
			if f.name() == name {
				return f
			}
		case <-deadline:
			t.Fatalf("%s: timed out waiting for %q after %s", c.nickname, name, window)
		}
	}
}

// ---------------------------------------------------------------------------
// concurrency
// ---------------------------------------------------------------------------

// Exercises the locking under -race: many players acting at once while rooms
// are created, joined, connected and left concurrently.
func TestConcurrentActivity(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	const players = 8
	clients := make([]*testClient, 0, players)
	for i := range players {
		clients = append(clients, h.join(fmt.Sprintf("p%d", i)))
	}

	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Go(func() {
			for i := range 20 {
				_ = c.send(dtos.SendMessageAction, fmt.Sprintf("%s says %d", c.nickname, i))
				_ = c.send(dtos.ReadyAction, "")
				_ = c.send(dtos.LieAction, fmt.Sprintf("answer from %s", c.nickname))
				_ = c.send(dtos.VoteAction, "rayleigh scattering")
			}
		})
	}

	// Churn the room maps at the same time.
	wg.Go(func() {
		for i := range 20 {
			room, err := h.svc.CreateRoom(uint(100 + i))
			if err != nil {
				continue
			}
			p, err := h.svc.Join(room.Code, &model.Player{Nickname: "drifter", UserId: uint(200 + i)})
			if err != nil {
				continue
			}
			bus := make(chan []byte, busSize)
			if _, err := h.svc.Connect(room.Code, p.Token, bus); err != nil {
				continue
			}
			h.svc.Leave(room.Code, p.Token)
		}
	})

	// Drain every bus so nothing wedges on a full channel.
	stop := make(chan struct{})
	for _, c := range clients {
		go func() {
			for {
				select {
				case <-c.bus:
				case <-stop:
					return
				}
			}
		}()
	}

	wg.Wait()
	close(stop)

	// The service is still responsive afterwards.
	if _, err := h.svc.CreateRoom(999); err != nil {
		t.Fatalf("service unusable after concurrent load: %v", err)
	}
}
