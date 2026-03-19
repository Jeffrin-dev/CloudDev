package persist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := State{
		S3: S3State{
			Buckets: map[string]S3BucketState{
				"my-bucket": {
					Objects: map[string]S3ObjectState{
						"hello.txt": {Data: []byte("hello"), ETag: "etag-1"},
					},
				},
			},
		},
		DynamoDB: DynamoDBState{
			Tables: map[string]DynamoDBTableState{
				"Users": {
					HashKey: "id",
					Items: map[string]map[string]interface{}{
						`{"S":"1"}`: {
							"id":   map[string]interface{}{"S": "1"},
							"name": map[string]interface{}{"S": "Ada"},
						},
					},
				},
			},
		},
	}

	if err := Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path := filepath.Join(home, ".clouddev", "state.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	var got State
	if err := Load(&got); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(got.S3.Buckets) != 1 || len(got.DynamoDB.Tables) != 1 {
		t.Fatalf("unexpected state after roundtrip: %#v", got)
	}
	if string(got.S3.Buckets["my-bucket"].Objects["hello.txt"].Data) != "hello" {
		t.Fatalf("unexpected s3 object after roundtrip: %#v", got.S3.Buckets["my-bucket"].Objects["hello.txt"])
	}
	if got.DynamoDB.Tables["Users"].HashKey != "id" {
		t.Fatalf("unexpected dynamodb hash key after roundtrip: %#v", got.DynamoDB.Tables["Users"])
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var got State
	if err := Load(&got); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
