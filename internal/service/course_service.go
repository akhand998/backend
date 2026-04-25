package service

import (
	"context"
	"errors"

	"github.com/Amanyd/backend/internal/domain"
	"github.com/Amanyd/backend/internal/port"
	"github.com/google/uuid"
)

type CourseService struct {
	courses port.CourseRepository
	lessons port.LessonRepository
}

func NewCourseService(courses port.CourseRepository, lessons port.LessonRepository) *CourseService {
	return &CourseService{courses: courses, lessons: lessons}
}

func (s *CourseService) Create(ctx context.Context, title, desc, rank string, instructorID uuid.UUID) (*domain.Course, error) {
	course := &domain.Course{
		Title:        title,
		Description:  desc,
		Rank:         rank,
		InstructorID: instructorID,
	}
	if err := s.courses.Create(ctx, course); err != nil {
		return nil, err
	}
	return course, nil
}

func (s *CourseService) GetByID(ctx context.Context, courseID uuid.UUID) (*domain.Course, error) {
	return s.courses.GetByID(ctx, courseID)
}

func (s *CourseService) ListByRank(ctx context.Context, rank string) ([]domain.Course, error) {
	return s.courses.ListByRank(ctx, rank)
}

func (s *CourseService) ListByInstructor(ctx context.Context, instructorID uuid.UUID) ([]domain.Course, error) {
	return s.courses.ListByInstructor(ctx, instructorID)
}

func (s *CourseService) Update(ctx context.Context, courseID, instructorID uuid.UUID, title, desc, rank string) (*domain.Course, error) {
	course, err := s.courses.GetByID(ctx, courseID)
	if err != nil {
		return nil, err
	}
	if course.InstructorID != instructorID {
		return nil, domain.ErrForbidden
	}

	course.Title = title
	course.Description = desc
	course.Rank = rank
	if err := s.courses.Update(ctx, course); err != nil {
		return nil, err
	}
	return course, nil
}

func (s *CourseService) Delete(ctx context.Context, courseID, instructorID uuid.UUID) error {
	course, err := s.courses.GetByID(ctx, courseID)
	if err != nil {
		return err
	}
	if course.InstructorID != instructorID {
		return domain.ErrForbidden
	}
	return s.courses.Delete(ctx, courseID)
}

// Lesson methods

func (s *CourseService) CreateLesson(ctx context.Context, courseID, instructorID uuid.UUID, title string, orderIdx int) (*domain.Lesson, error) {
	if err := s.verifyOwnership(ctx, courseID, instructorID); err != nil {
		return nil, err
	}

	lesson := &domain.Lesson{
		CourseID:  courseID,
		Title:    title,
		OrderIdx: orderIdx,
	}
	if err := s.lessons.Create(ctx, lesson); err != nil {
		return nil, err
	}
	return lesson, nil
}

func (s *CourseService) ListLessons(ctx context.Context, courseID uuid.UUID) ([]domain.Lesson, error) {
	return s.lessons.ListByCourse(ctx, courseID)
}

func (s *CourseService) UpdateLesson(ctx context.Context, lessonID, instructorID uuid.UUID, title string, orderIdx int) (*domain.Lesson, error) {
	lesson, err := s.lessons.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, lesson.CourseID, instructorID); err != nil {
		return nil, err
	}

	lesson.Title = title
	lesson.OrderIdx = orderIdx
	if err := s.lessons.Update(ctx, lesson); err != nil {
		return nil, err
	}
	return lesson, nil
}

func (s *CourseService) DeleteLesson(ctx context.Context, lessonID, instructorID uuid.UUID) error {
	lesson, err := s.lessons.GetByID(ctx, lessonID)
	if err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, lesson.CourseID, instructorID); err != nil {
		return err
	}
	return s.lessons.Delete(ctx, lessonID)
}

func (s *CourseService) verifyOwnership(ctx context.Context, courseID, instructorID uuid.UUID) error {
	course, err := s.courses.GetByID(ctx, courseID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	if course.InstructorID != instructorID {
		return domain.ErrForbidden
	}
	return nil
}
