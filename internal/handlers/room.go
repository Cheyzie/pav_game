package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Cheyzie/pav_game/internal/dtos"
	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/Cheyzie/pav_game/internal/service"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/websocket"
)

func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserId(r)
	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}

	var input dtos.CreateRoomInput
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&input); err != nil {
		newErrorResponse(w, http.StatusBadRequest, "Invalid input", err)
		return
	}

	if err := h.validator.Struct(input); err != nil {
		var errs validator.ValidationErrors
		if errors.As(err, &errs) {
			newErrorResponse(w, http.StatusBadRequest, strings.Join(formatValidationErrors(errs), "|"), err)
			return
		}
		newErrorResponse(w, http.StatusBadRequest, "Invalid input", err)
		return
	}

	room, err := h.gameService.CreateRoom(input.PromptsWrittenIn, userID)

	if err != nil {
		newErrorResponse(w, http.StatusUnprocessableEntity, err.Error(), err)
		return
	}

	newResponse(w, http.StatusCreated, map[string]any{"code": room.Code})
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	userId, err := getUserId(r)
	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}
	code := r.PathValue("code")
	input := &dtos.JoinPlayerInput{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&input); err != nil {
		newErrorResponse(w, http.StatusBadRequest, "Invalid input", err)
		return
	}
	player := &model.Player{Nickname: input.Nickname, UserId: userId}
	player, err = h.gameService.Join(code, player)
	if err != nil {
		newErrorResponse(w, http.StatusUnprocessableEntity, err.Error(), err)
		return
	}

	newResponse(w, http.StatusCreated, map[string]any{"nickname": player.Nickname, "token": player.Token})
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	connection, _ := h.upgrader.Upgrade(w, r, nil)
	defer connection.Close()
	var player *model.Player

	input := &dtos.ConnectPlayerInput{}
	bus := make(chan []byte, 5)
	code := ""
	done := make(chan struct{})
	go func() {
		for {
			select {
			case message := <-bus:
				connection.WriteMessage(websocket.TextMessage, message)
			case <-done:
				return
			}
		}
	}()
	defer func() {
		if player != nil {
			h.gameService.Leave(code, player.Token)
		}
		close(done)
	}()

	for {
		mt, message, err := connection.ReadMessage()

		if err != nil || mt == websocket.CloseMessage {
			break
		}

		if err := json.Unmarshal(message, &input); err != nil {
			connection.WriteJSON(service.ErrFrame("Invalid input"))
			continue
		}

		player, err = h.gameService.Connect(input.Code, input.Token, bus)
		if err != nil {
			connection.WriteJSON(service.ErrFrame(err.Error()))
			continue
		}
		code = input.Code
		break
	}
	if player == nil {
		return
	}
	for {
		mt, message, err := connection.ReadMessage()

		if err != nil || mt == websocket.CloseMessage {
			h.gameService.Leave(code, input.Token)
			break
		}
		action := dtos.Action{}
		if err := json.Unmarshal(message, &action); err != nil {
			continue
		}
		h.gameService.Dispatch(
			input.Code,
			player.Nickname,
			action,
		)
	}
}
