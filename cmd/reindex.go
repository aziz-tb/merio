package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	enumspb "go.temporal.io/api/enums/v1"

	"github.com/aziz-tb/merio/internal/temporal"
	wfembeddings "github.com/aziz-tb/merio/internal/workflows/embeddings"
)

// runReindexEmbeddings is invoked from main when --reindex-embeddings is
// passed. Kicks off the BulkReindexWorkflow on the embeddings queue and
// returns the workflow id so an operator can monitor in Temporal UI.
//
// Pre-conditions: Temporal must be enabled in config; the embeddings
// worker must be running (otherwise the workflow sits in the queue
// indefinitely).
//
// Usage:
//
//	./merio --reindex-embeddings \
//	    --reindex-content-type=kb_chunk \
//	    --reindex-batch-size=100 \
//	    --reindex-parallelism=4
//
// The content type defaults to 'kb_chunk' (the only one F12 ships).
func runReindexEmbeddings(tc *temporal.Client) {
	if tc == nil {
		log.Fatalf("reindex: Temporal client is nil (temporal.enabled=false or dial failed)")
	}

	contentType := ko.String("reindex-content-type")
	if contentType == "" {
		contentType = "kb_chunk"
	}
	batchSize := ko.Int("reindex-batch-size")
	if batchSize <= 0 {
		batchSize = 100
	}
	parallelism := ko.Int("reindex-parallelism")
	if parallelism <= 0 {
		parallelism = 4
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	workflowID := fmt.Sprintf("bulk-reindex:%s:%d", contentType, time.Now().Unix())
	run, err := tc.StartWorkflow(ctx, temporal.StartOptions{
		ID:        workflowID,
		TaskQueue: temporal.QueueEmbeddings,
		SearchAttributes: map[string]any{
			temporal.SAWorkflowKind: "embedding",
			temporal.SAEntityType:   "bulk-reindex",
		},
		ReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
	}, wfembeddings.BulkReindexWorkflow, wfembeddings.BulkReindexInput{
		ContentType: contentType,
		BatchSize:   batchSize,
		Parallelism: parallelism,
		TraceID:     workflowID,
	})
	if err != nil {
		log.Fatalf("reindex: start workflow: %v", err)
	}

	fmt.Printf("Bulk reindex started.\n")
	fmt.Printf("  workflow_id = %s\n", run.GetID())
	fmt.Printf("  run_id      = %s\n", run.GetRunID())
	fmt.Printf("Monitor in Temporal UI: http://127.0.0.1:8080\n")
	os.Exit(0)
}
