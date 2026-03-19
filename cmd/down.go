package cmd

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/clouddev/clouddev/internal/config"
	"github.com/clouddev/clouddev/internal/docker"
	"github.com/clouddev/clouddev/internal/persist"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all running local cloud services",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig("clouddev.yml")
		if err != nil {
			return err
		}

		printVerbose("Stopping service orchestration in stub mode")
		printInfo("Stopping local cloud services...")
		printWarning("Stub mode: Docker/service shutdown not implemented yet")
		printSuccess("Status: local services shutdown stub completed")
		state, err := collectState(cfg)
		if err != nil {
			return err
		}
		if err := persist.Save(state); err != nil {
			return err
		}
		manager, err := docker.NewManager(os.Stdout)
		if err != nil {
			return err
		}

		if err := manager.StopAll(context.Background()); err != nil {
			return err
		}

		printSuccess("Stopped all managed clouddev containers")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func collectState(cfg *config.Config) (persist.State, error) {
	state := persist.State{
		S3:       persist.S3State{Buckets: make(map[string]persist.S3BucketState)},
		DynamoDB: persist.DynamoDBState{Tables: make(map[string]persist.DynamoDBTableState)},
	}

	if cfg.Services.S3 {
		s3State, err := collectS3State(cfg.Ports.S3)
		if err != nil {
			return persist.State{}, err
		}
		state.S3 = s3State
	}
	if cfg.Services.DynamoDB {
		dynamoState, err := collectDynamoDBState(cfg.Ports.DynamoDB)
		if err != nil {
			return persist.State{}, err
		}
		state.DynamoDB = dynamoState
	}

	return state, nil
}

func collectS3State(port int) (persist.S3State, error) {
	type bucketList struct {
		Buckets struct {
			Bucket []struct {
				Name string `xml:"Name"`
			} `xml:"Bucket"`
		} `xml:"Buckets"`
	}
	type objectList struct {
		Contents []struct {
			Key string `xml:"Key"`
		} `xml:"Contents"`
	}

	state := persist.S3State{Buckets: make(map[string]persist.S3BucketState)}
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return persist.S3State{}, fmt.Errorf("collect s3 state: %w", err)
	}
	defer resp.Body.Close()

	var buckets bucketList
	if err := xml.NewDecoder(resp.Body).Decode(&buckets); err != nil {
		return persist.S3State{}, fmt.Errorf("decode s3 buckets: %w", err)
	}

	for _, bucket := range buckets.Buckets.Bucket {
		bucketState := persist.S3BucketState{Objects: make(map[string]persist.S3ObjectState)}
		objectsResp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/%s", port, bucket.Name))
		if err != nil {
			return persist.S3State{}, fmt.Errorf("list s3 objects for %s: %w", bucket.Name, err)
		}

		var objects objectList
		if err := xml.NewDecoder(objectsResp.Body).Decode(&objects); err != nil {
			objectsResp.Body.Close()
			return persist.S3State{}, fmt.Errorf("decode s3 objects for %s: %w", bucket.Name, err)
		}
		objectsResp.Body.Close()

		for _, entry := range objects.Contents {
			objectResp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/%s/%s", port, bucket.Name, entry.Key))
			if err != nil {
				return persist.S3State{}, fmt.Errorf("get s3 object %s/%s: %w", bucket.Name, entry.Key, err)
			}
			body, err := io.ReadAll(objectResp.Body)
			objectResp.Body.Close()
			if err != nil {
				return persist.S3State{}, fmt.Errorf("read s3 object %s/%s: %w", bucket.Name, entry.Key, err)
			}

			bucketState.Objects[entry.Key] = persist.S3ObjectState{
				Data: body,
				ETag: objectResp.Header.Get("ETag"),
			}
		}
		state.Buckets[bucket.Name] = bucketState
	}

	return state, nil
}

func collectDynamoDBState(port int) (persist.DynamoDBState, error) {
	type listTablesResponse struct {
		TableNames []string `json:"TableNames"`
	}
	type describeTableResponse struct {
		Table struct {
			KeySchema []struct {
				AttributeName string `json:"AttributeName"`
				KeyType       string `json:"KeyType"`
			} `json:"KeySchema"`
		} `json:"Table"`
	}
	type scanResponse struct {
		Items []map[string]interface{} `json:"Items"`
	}

	state := persist.DynamoDBState{Tables: make(map[string]persist.DynamoDBTableState)}

	var tables listTablesResponse
	if err := dynamoRequest(port, "DynamoDB_20120810.ListTables", map[string]interface{}{}, &tables); err != nil {
		return persist.DynamoDBState{}, fmt.Errorf("collect dynamodb state: %w", err)
	}

	for _, tableName := range tables.TableNames {
		var description describeTableResponse
		if err := dynamoRequest(port, "DynamoDB_20120810.DescribeTable", map[string]interface{}{"TableName": tableName}, &description); err != nil {
			return persist.DynamoDBState{}, fmt.Errorf("describe dynamodb table %s: %w", tableName, err)
		}

		hashKey := ""
		for _, key := range description.Table.KeySchema {
			if key.KeyType == "HASH" {
				hashKey = key.AttributeName
				break
			}
		}

		var scan scanResponse
		if err := dynamoRequest(port, "DynamoDB_20120810.Scan", map[string]interface{}{"TableName": tableName}, &scan); err != nil {
			return persist.DynamoDBState{}, fmt.Errorf("scan dynamodb table %s: %w", tableName, err)
		}

		items := make(map[string]map[string]interface{}, len(scan.Items))
		for _, item := range scan.Items {
			keyValue, ok := item[hashKey]
			if !ok {
				continue
			}
			keyBytes, err := json.Marshal(keyValue)
			if err != nil {
				return persist.DynamoDBState{}, fmt.Errorf("marshal dynamodb item key for %s: %w", tableName, err)
			}
			items[string(keyBytes)] = item
		}

		state.Tables[tableName] = persist.DynamoDBTableState{
			HashKey: hashKey,
			Items:   items,
		}
	}

	return state, nil
}

func dynamoRequest(port int, target string, payload map[string]interface{}, dest interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d", port), strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", target)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(dest)
}
