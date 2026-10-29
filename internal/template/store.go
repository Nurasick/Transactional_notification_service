package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"text/template"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	client     *minio.Client
	bucketName string
}

func New(endpoint, accessKey, secretKey, bucketName string, useSSL bool) (*Store, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: client, bucketName: bucketName}, nil
}

// Render fetches the template for eventType from MinIO and renders it with data.
func (s *Store) Render(ctx context.Context, eventType string, data map[string]any) (string, error) {
	objectName := fmt.Sprintf("%s.html", eventType)

	obj, err := s.client.GetObject(ctx, s.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("fetch template %s: %w", objectName, err)
	}
	defer obj.Close()

	raw, err := io.ReadAll(obj)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", objectName, err)
	}

	return render(raw, objectName, data)
}

// Parses vals into template and render/executes the template
func render(raw []byte, name string, data map[string]any) (string, error) {

	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.String(), nil
}
