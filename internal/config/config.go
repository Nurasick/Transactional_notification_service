package config

import (
	"fmt"
	"os"
)

type Config struct {
	RabbitMQ RabbitMQConfig
	Redis    RedisConfig
	Minio    MinIOConfig
	Postgres PostgresConfig
}

type RabbitMQConfig struct {
	URL string
}

type RedisConfig struct {
	Addr string
}

type PostgresConfig struct {
	DSN string
}

type MinIOConfig struct {
	Endpoint       string
	AccessKey       string
	SecretAccessKey string
	UseSSL          bool
	BucketName      string
}

func Load() (*Config, error) {
	rabbitURL, err := required("RABBITMQ_URL")
	if err != nil{
		return nil, err
	}
	redisAddr, err := required("REDIS_ADDR")
	if err != nil{
		return nil, err
	}
	postgresDSN, err := required("POSTGRES_DSN")
	if err != nil{
		return nil, err
	}
	minioEndpoint, err := required("MINIO_ENDPOINT")
	if err != nil{
		return nil, err
	}
	minioAccessKey, err := required("MINIO_ACCESS_KEY")
	if err != nil{
		return nil, err
	}
	minioSecretKey, err := required("MINIO_SECRET_KEY")
	if err != nil{
		return nil, err
	}
	minioUseSSL := os.Getenv("MINIO_USE_SSL") == "true"

	minioBucketName, err := required("MINIO_BUCKET_NAME")
	if err != nil{
		return nil, err
	}

	return &Config{
		RabbitMQ: RabbitMQConfig{
			URL: rabbitURL,
		},
		Redis: RedisConfig{
			Addr: redisAddr,
		},
		Postgres: PostgresConfig{
			DSN: postgresDSN,
		},
		Minio: MinIOConfig{
			Endpoint:        minioEndpoint,
			AccessKey:       minioAccessKey,
			SecretAccessKey: minioSecretKey,
			UseSSL:          minioUseSSL,
			BucketName:      minioBucketName,
		},
	}, nil



}

func required(key string)(string, error) {
	v := os.Getenv(key)
	if v ==""{
		return "", fmt.Errorf("missing required environment vairable: %s", key)
	}
	return v, nil
}