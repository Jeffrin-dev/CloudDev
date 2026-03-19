package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	S3       S3State       `json:"s3"`
	DynamoDB DynamoDBState `json:"dynamodb"`
}

type S3State struct {
	Buckets map[string]S3BucketState `json:"buckets"`
}

type S3BucketState struct {
	Objects map[string]S3ObjectState `json:"objects"`
}

type S3ObjectState struct {
	Data []byte `json:"data,omitempty"`
	ETag string `json:"etag,omitempty"`
}

type DynamoDBState struct {
	Tables map[string]DynamoDBTableState `json:"tables"`
}

type DynamoDBTableState struct {
	HashKey string                            `json:"hash_key,omitempty"`
	Items   map[string]map[string]interface{} `json:"items"`
}

func Save(state interface{}) error {
	path, err := stateFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

func Load(dest interface{}) error {
	path, err := stateFilePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("parse state file: %w", err)
	}
	return nil
}

func stateFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".clouddev", "state.json"), nil
}
