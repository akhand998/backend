package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Amanyd/backend/internal/domain"
	"github.com/Amanyd/backend/internal/infra/nats"
	"github.com/Amanyd/backend/internal/port"
	"github.com/google/uuid"
)

const uploadURLExpiry = 15 * time.Minute
const viewURLExpiry = 1 * time.Hour

type FileService struct {
	files   port.FileRepository
	lessons port.LessonRepository
	courses port.CourseRepository
	storage port.ObjectStorage
	queue   port.MessageQueue
	bucket  string
}

func NewFileService(
	files port.FileRepository,
	lessons port.LessonRepository,
	courses port.CourseRepository,
	storage port.ObjectStorage,
	queue port.MessageQueue,
	bucket string,
) *FileService {
	return &FileService{
		files:   files,
		lessons: lessons,
		courses: courses,
		storage: storage,
		queue:   queue,
		bucket:  bucket,
	}
}

type UploadResult struct {
	FileID    uuid.UUID `json:"file_id"`
	UploadURL string    `json:"upload_url"`
}

func (s *FileService) InitUpload(ctx context.Context, lessonID uuid.UUID, fileName string, fileType domain.FileType, instructorID uuid.UUID) (*UploadResult, error) {
	lesson, err := s.lessons.GetByID(ctx, lessonID)
	if err != nil {
		return nil, err
	}

	course, err := s.courses.GetByID(ctx, lesson.CourseID)
	if err != nil {
		return nil, err
	}
	if course.InstructorID != instructorID {
		return nil, domain.ErrForbidden
	}

	file := &domain.FileAsset{
		LessonID:     lessonID,
		FileName:     fileName,
		FileType:     fileType,
		MinioKey:     fmt.Sprintf("courses/%s/%s/%s", course.ID, uuid.New(), fileName),
		IngestStatus: domain.IngestPending,
	}
	if err := s.files.Create(ctx, file); err != nil {
		return nil, err
	}

	url, err := s.storage.PresignUpload(ctx, file.MinioKey, uploadURLExpiry)
	if err != nil {
		return nil, fmt.Errorf("presign upload: %w", err)
	}

	return &UploadResult{FileID: file.ID, UploadURL: url}, nil
}

func (s *FileService) ConfirmUpload(ctx context.Context, fileID, instructorID uuid.UUID) error {
	file, err := s.files.GetByID(ctx, fileID)
	if err != nil {
		return err
	}

	lesson, err := s.lessons.GetByID(ctx, file.LessonID)
	if err != nil {
		return err
	}

	course, err := s.courses.GetByID(ctx, lesson.CourseID)
	if err != nil {
		return err
	}
	if course.InstructorID != instructorID {
		return domain.ErrForbidden
	}

	payload, err := json.Marshal(map[string]string{
		"bucket":     s.bucket,
		"key":        file.MinioKey,
		"course_id":  course.ID.String(),
		"file_id":    file.ID.String(),
		"file_name":  file.FileName,
		"teacher_id": instructorID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal ingest payload: %w", err)
	}

	if err := s.queue.Publish(ctx, nats.SubjectIngestRequest, payload); err != nil {
		return fmt.Errorf("publish ingest request: %w", err)
	}

	return s.files.UpdateIngestStatus(ctx, fileID, domain.IngestProcessing)
}

func (s *FileService) GetIngestStatus(ctx context.Context, fileID uuid.UUID) (domain.IngestStatus, error) {
	file, err := s.files.GetByID(ctx, fileID)
	if err != nil {
		return "", err
	}
	return file.IngestStatus, nil
}

func (s *FileService) GetViewURL(ctx context.Context, fileID uuid.UUID) (string, error) {
	file, err := s.files.GetByID(ctx, fileID)
	if err != nil {
		return "", err
	}
	return s.storage.PresignView(ctx, file.MinioKey, viewURLExpiry)
}

func (s *FileService) ListByLesson(ctx context.Context, lessonID uuid.UUID) ([]domain.FileAsset, error) {
	return s.files.ListByLesson(ctx, lessonID)
}
