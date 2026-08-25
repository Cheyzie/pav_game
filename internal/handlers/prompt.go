package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/Cheyzie/pav_game/internal/dtos"
	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/go-playground/validator/v10"
)

var allowedLangs = []string{"ua", "en"}

func (h *Handler) createPrompt(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserId(r)

	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}

	var input dtos.PromptCreateInput
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&input); err != nil {
		newErrorResponse(w, http.StatusUnprocessableEntity, "Invalid input. Expected written_in, question, truth and category", err)
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
	prompt := model.Prompt{
		UserID:    userID,
		WrittenIn: input.WrittenIn,
		Question:  input.Question,
		Truth:     input.Truth,
		Category:  input.Category,
	}

	if err := h.promptService.Create(&prompt); err != nil {
		newErrorResponse(w, http.StatusUnprocessableEntity, "create prompt error", err)
		return
	}

	newResponse(w, http.StatusCreated, map[string]any{"id": prompt.ID})
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	_, err := getUserId(r)
	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}
	writtenIn := r.URL.Query().Get("writtenIn")
	if !slices.Contains(allowedLangs, writtenIn) {
		newErrorResponse(w, http.StatusBadRequest, "writtenIn must be one of "+strings.Join(allowedLangs, ", "), errors.New("unsupported writtenIn"))
		return
	}
	categories, err := h.promptService.GetCategories(writtenIn)
	if err != nil {
		newErrorResponse(w, http.StatusInternalServerError, "cant get categories", err)
		return
	}
	newResponse(w, http.StatusOK, categories)
}

func (h *Handler) promptsCountByUser(w http.ResponseWriter, r *http.Request) {
	id, err := getUserId(r)
	if err != nil {
		newErrorResponse(w, http.StatusUnauthorized, "cant resolve user id", err)
		return
	}

	count, err := h.promptService.CountByUser(id)
	if err != nil {
		newErrorResponse(w, http.StatusInternalServerError, "cant get sessions", err)
		return
	}
	newResponse(w, http.StatusOK, map[string]any{"count": count})
}
