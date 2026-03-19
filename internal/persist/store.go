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

func (s *State) UnmarshalJSON(data []byte) error {
	type rawState struct {
		S3       json.RawMessage `json:"s3"`
		DynamoDB json.RawMessage `json:"dynamodb"`
	}

	var raw rawState
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s3State, err := decodeS3State(raw.S3)
	if err != nil {
		return err
	}
	dynamoState, err := decodeDynamoDBState(raw.DynamoDB)
	if err != nil {
		return err
	}

	s.S3 = s3State
	s.DynamoDB = dynamoState
	return nil
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

func decodeS3State(data json.RawMessage) (S3State, error) {
	if len(data) == 0 || string(data) == "null" {
		return S3State{}, nil
	}

	var structured S3State
	if err := json.Unmarshal(data, &structured); err == nil && structured.Buckets != nil {
		return structured, nil
	}

	var raw map[string]map[string][]byte
	if err := json.Unmarshal(data, &raw); err != nil {
		return S3State{}, err
	}

	state := S3State{Buckets: make(map[string]S3BucketState, len(raw))}
	for bucketName, objects := range raw {
		bucketState := S3BucketState{Objects: make(map[string]S3ObjectState, len(objects))}
		for key, objectData := range objects {
			bucketState.Objects[key] = S3ObjectState{Data: objectData}
		}
		state.Buckets[bucketName] = bucketState
	}
	return state, nil
}

func decodeDynamoDBState(data json.RawMessage) (DynamoDBState, error) {
	if len(data) == 0 || string(data) == "null" {
		return DynamoDBState{}, nil
	}

	var structured DynamoDBState
	if err := json.Unmarshal(data, &structured); err == nil && structured.Tables != nil {
		return structured, nil
	}

	var raw map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return DynamoDBState{}, err
	}

	state := DynamoDBState{Tables: make(map[string]DynamoDBTableState, len(raw))}
	for tableName, items := range raw {
		tableState := DynamoDBTableState{Items: make(map[string]map[string]interface{})}
		for itemKey, item := range items {
			if itemKey == "__clouddev_hash_key" {
				if name, ok := item["name"].(string); ok {
					tableState.HashKey = name
				}
				continue
			}
			tableState.Items[itemKey] = item
		}
		state.Tables[tableName] = tableState
	}
	return state, nil
}
