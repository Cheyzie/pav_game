package service

import (
	"fmt"

	"github.com/Cheyzie/pav_game/internal/model"
)

type PromptRepository interface {
	Store(room *model.Prompt) error
	GetRand(writtenIn string, usedID []uint) (model.Prompt, error)
	GetCategories(writtenIn string) ([]model.Category, error)
	CountByUser(userID uint) (uint, error)
	CountByWrittenIn(writtenIn string) (uint, error)
}

type PromptService struct {
	repo PromptRepository
}

func NewPromptService(repo PromptRepository) *PromptService {
	return &PromptService{
		repo: repo,
	}
}

func (s *PromptService) Create(prompt *model.Prompt) error {

	if err := s.repo.Store(prompt); err != nil {
		return fmt.Errorf("store prompt error: %w", err)
	}

	return nil
}

func (s *PromptService) GetCategories(writtenIn string) ([]model.Category, error) {
	categories, err := s.repo.GetCategories(writtenIn)

	if err != nil {
		return categories, fmt.Errorf("get categories by written_in=%s error: %w", writtenIn, err)
	}

	return categories, nil
}

func (s *PromptService) CountByUser(userID uint) (uint, error) {
	count, err := s.repo.CountByUser(userID)

	if err != nil {
		return count, fmt.Errorf("get prompts count by user_id=%d error: %w", userID, err)
	}

	return count, nil
}
