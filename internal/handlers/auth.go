package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Cheyzie/pav_game/internal/dtos"
	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/go-playground/validator/v10"
)

func (h *Handler) signIn(w http.ResponseWriter, r *http.Request) {
	var input dtos.SigninInput
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&input); err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "Invalid input. Expected email and password", err)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		var errs validator.ValidationErrors
		if errors.As(err, &errs) {
			newErrorResponse(w, http.StatusBadRequest, strings.Join(formatValidationErrors(errs), "|"), err)
			return
		}
		newErrorResponse(w, http.StatusUnprocessableEntity, "invalid input", err)
		return
	}

	ipAddress := strings.TrimRight(r.RemoteAddr, ":")

	token, err := h.authService.GenerateToken(input.Email, input.Password, input.SessionName, ipAddress)
	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "Invalid credentials", err)
		return
	}
	newResponse(w, http.StatusOK, token)

}

func (h *Handler) userSessions(w http.ResponseWriter, r *http.Request) {
	id, err := getUserId(r)

	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}
	sessions, err := h.authService.ListUserSessions(id)
	if err != nil {
		newErrorResponse(w, http.StatusInternalServerError, "cant get sessions", err)
		return
	}
	newResponse(w, http.StatusOK, sessions)
}

func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) {
	var input dtos.RefreshInput
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&input); err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "Invalid input. Expected refresh_token", err)
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
	ipAddress := r.RemoteAddr

	token, err := h.authService.RefreshToken(input.RefreshToken, ipAddress)
	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "Invalid credentials", err)
		return
	}
	newResponse(w, http.StatusOK, token)

}

func (h *Handler) signUp(w http.ResponseWriter, r *http.Request) {
	var input dtos.SignupInput
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&input); err != nil {
		newErrorResponse(w, http.StatusUnprocessableEntity, "Invalid input. Expected username, email and password", err)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		var errs validator.ValidationErrors
		if errors.As(err, &errs) {
			newErrorResponse(w, http.StatusBadRequest, strings.Join(formatValidationErrors(errs), "|"), err)
			return
		}
		newErrorResponse(w, http.StatusUnprocessableEntity, "invalid input", err)
		return
	}
	user := model.User{
		Username: input.Username,
		Email:    input.Email,
		Password: input.Password,
	}
	id, err := h.authService.CreateUser(user)
	if err != nil {
		newErrorResponse(w, http.StatusBadRequest, "Invalid data", err)
		return
	}
	user.ID = id

	newResponse(w, http.StatusCreated, user)
}

func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	id, err := getUserId(r)

	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}
	user, err := h.authService.GetByID(id)
	if err != nil {
		newErrorResponse(w, http.StatusInternalServerError, "cant find user by id", err)
		return
	}

	newResponse(w, http.StatusOK, user)
}

func (h *Handler) dropSession(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserId(r)

	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}

	sessionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		newErrorResponse(w, http.StatusBadRequest, "cant parse session id", err)
		return
	}

	if err := h.authService.DropSession(userID, uint(sessionID)); err != nil {
		newErrorResponse(w, http.StatusInternalServerError, "cant drop sessions", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserId(r)

	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}

	sessionID, err := getSessionId(r)
	if err != nil {
		newErrorResponse(w, http.StatusBadRequest, "cant parse session id", err)
		return
	}

	if err := h.authService.DropSession(userID, uint(sessionID)); err != nil {
		newErrorResponse(w, http.StatusInternalServerError, "cant drop sessions", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
