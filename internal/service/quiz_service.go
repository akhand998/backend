package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Amanyd/backend/internal/domain"
	"github.com/Amanyd/backend/internal/infra/nats"
	"github.com/Amanyd/backend/internal/port"
	"github.com/google/uuid"
)

type QuizService struct {
	quizzes port.QuizRepository
	courses port.CourseRepository
	queue   port.MessageQueue
}

func NewQuizService(quizzes port.QuizRepository, courses port.CourseRepository, queue port.MessageQueue) *QuizService {
	return &QuizService{quizzes: quizzes, courses: courses, queue: queue}
}

func (s *QuizService) ListByCourse(ctx context.Context, courseID uuid.UUID) ([]domain.Quiz, error) {
	return s.quizzes.ListQuizzesByCourse(ctx, courseID)
}

type QuizWithQuestions struct {
	Quiz      domain.Quiz
	Questions []domain.Question
}

func (s *QuizService) GetQuiz(ctx context.Context, quizID uuid.UUID) (*QuizWithQuestions, error) {
	quiz, err := s.quizzes.GetQuizByID(ctx, quizID)
	if err != nil {
		return nil, err
	}

	questions, err := s.quizzes.ListQuestionsByQuiz(ctx, quizID)
	if err != nil {
		return nil, err
	}

	return &QuizWithQuestions{Quiz: *quiz, Questions: questions}, nil
}

func (s *QuizService) StartAttempt(ctx context.Context, quizID, userID uuid.UUID) (*domain.Attempt, error) {
	if _, err := s.quizzes.GetQuizByID(ctx, quizID); err != nil {
		return nil, err
	}

	attempt := &domain.Attempt{
		QuizID: quizID,
		UserID: userID,
	}
	if err := s.quizzes.CreateAttempt(ctx, attempt); err != nil {
		return nil, err
	}
	return attempt, nil
}

func (s *QuizService) SubmitAnswer(ctx context.Context, attemptID, questionID uuid.UUID, userAnswer string) (*domain.Answer, error) {
	question, err := s.quizzes.GetQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}

	isCorrect := userAnswer == question.Answer

	answer := &domain.Answer{
		AttemptID:  attemptID,
		QuestionID: questionID,
		UserAnswer: userAnswer,
		IsCorrect:  isCorrect,
	}
	if err := s.quizzes.CreateAnswer(ctx, answer); err != nil {
		return nil, err
	}
	return answer, nil
}

func (s *QuizService) FinishAttempt(ctx context.Context, attemptID uuid.UUID) (*domain.Attempt, error) {
	attempt, err := s.quizzes.GetAttemptByID(ctx, attemptID)
	if err != nil {
		return nil, err
	}

	answers, err := s.quizzes.ListAnswersByAttempt(ctx, attemptID)
	if err != nil {
		return nil, err
	}

	var correct int
	for _, a := range answers {
		if a.IsCorrect {
			correct++
		}
	}

	total := len(answers)
	var score float64
	if total > 0 {
		score = float64(correct) / float64(total) * 100
	}

	now := attempt.StartedAt // will be overwritten
	attempt.Score = score
	attempt.Total = total
	attempt.EndedAt = &now

	if err := s.quizzes.UpdateAttempt(ctx, attempt); err != nil {
		return nil, err
	}
	return attempt, nil
}

type AttemptResults struct {
	Attempt domain.Attempt
	Answers []domain.Answer
}

func (s *QuizService) GetResults(ctx context.Context, attemptID, userID uuid.UUID) (*AttemptResults, error) {
	attempt, err := s.quizzes.GetAttemptByID(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt.UserID != userID {
		return nil, domain.ErrForbidden
	}

	answers, err := s.quizzes.ListAnswersByAttempt(ctx, attemptID)
	if err != nil {
		return nil, err
	}

	return &AttemptResults{Attempt: *attempt, Answers: answers}, nil
}

func (s *QuizService) ResetQuiz(ctx context.Context, quizID, instructorID uuid.UUID) error {
	quiz, err := s.quizzes.GetQuizByID(ctx, quizID)
	if err != nil {
		return err
	}

	course, err := s.courses.GetByID(ctx, quiz.CourseID)
	if err != nil {
		return err
	}
	if course.InstructorID != instructorID {
		return domain.ErrForbidden
	}

	if err := s.quizzes.DeleteQuestionsByQuiz(ctx, quizID); err != nil {
		return err
	}

	if err := s.quizzes.UpdateQuizStatus(ctx, quizID, domain.QuizGenerating); err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"course_id":    course.ID.String(),
		"difficulty":   string(quiz.Difficulty),
		"limit_chunks": 20,
	})
	if err != nil {
		return fmt.Errorf("marshal quiz request: %w", err)
	}

	return s.queue.Publish(ctx, nats.SubjectQuizRequest, payload)
}
