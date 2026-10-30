package config_test

import (
	"testing"

	"github.com/Nurasick/transactional-notification-service/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAll(t *testing.T) {
	t.Helper()
	t.Setenv("RABBITMQ_URL", "amqp://u:p@localhost:5672/")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("POSTGRES_DSN", "postgres://u:p@localhost:5433/db")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("MINIO_ACCESS_KEY", "ak")
	t.Setenv("MINIO_SECRET_KEY", "sk")
	t.Setenv("MINIO_BUCKET_NAME", "bucket")
}

func TestLoad_AllSet(t *testing.T) {
	setAll(t)
	t.Setenv("MINIO_USE_SSL", "true")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, "amqp://u:p@localhost:5672/", cfg.RabbitMQ.URL)
	assert.Equal(t, "postgres://u:p@localhost:5433/db", cfg.Postgres.DSN)
	assert.Equal(t, "localhost:9000", cfg.Minio.Endpoint)
	assert.Equal(t, "ak", cfg.Minio.AccessKey)
	assert.Equal(t, "sk", cfg.Minio.SecretAccessKey)
	assert.Equal(t, "bucket", cfg.Minio.BucketName)
	assert.True(t, cfg.Minio.UseSSL)

}

func TestLoad_MissingVar(t *testing.T) {

	VARIABLES := []string{"RABBITMQ_URL", "REDIS_ADDR", "POSTGRES_DSN", "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET_NAME"}

	for _, val := range VARIABLES {
		t.Run(val, func(t *testing.T) {
			setAll(t)
			t.Setenv(val, "")
			_, err := config.Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), val)
		})
	}
}

func TestLoad_MinioUseSsl(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true", "true", true},
		{"false", "false", false},
		{"empty", "", false},
		{"uppercase", "TRUE", false},
		{"one", "1", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setAll(t)
			t.Setenv("MINIO_USE_SSL", test.value)
			cfg, err := config.Load()
			require.NoError(t, err)
			assert.Equal(t, test.expected, cfg.Minio.UseSSL)
		})
	}
}
