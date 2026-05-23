package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"

	itemporal "github.com/abhinavxd/libredesk/internal/temporal"
	wfairuns "github.com/abhinavxd/libredesk/internal/workflows/airuns"
)

type inFlightAgentRun struct {
	TraceID    string
	WorkflowID string
	AgentID    int
	StartedAt  time.Time
}

func agentRunWorkflowID(traceID string) string {
	return "ai-run:" + traceID
}

func findRunningAgentRun(
	ctx context.Context,
	tc *itemporal.Client,
	conversationID int,
	agentID *int,
) (*inFlightAgentRun, error) {
	if tc == nil || conversationID <= 0 {
		return nil, nil
	}

	query := fmt.Sprintf(
		`ExecutionStatus="Running" AND %s="%s" AND %s="%s" AND %s="%d"`,
		itemporal.SAWorkflowKind, wfairuns.WorkflowKind,
		itemporal.SAEntityType, "conversation",
		itemporal.SAEntityID, conversationID,
	)
	if agentID != nil && *agentID > 0 {
		query += fmt.Sprintf(` AND %s="%d"`, itemporal.SAAgentID, *agentID)
	}

	resp, err := tc.SDK().ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: tc.Config().Namespace,
		PageSize:  1,
		Query:     query,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Executions) == 0 {
		return nil, nil
	}

	info := resp.Executions[0]
	run := &inFlightAgentRun{
		WorkflowID: info.Execution.GetWorkflowId(),
		StartedAt:  info.GetStartTime().AsTime(),
	}
	if strings.HasPrefix(run.WorkflowID, "ai-run:") {
		run.TraceID = strings.TrimPrefix(run.WorkflowID, "ai-run:")
	}
	run.AgentID = searchAttributeInt(info.SearchAttributes.GetIndexedFields()[itemporal.SAAgentID])
	if run.TraceID == "" {
		run.TraceID = searchAttributeString(info.SearchAttributes.GetIndexedFields()[itemporal.SATraceID])
	}
	return run, nil
}

func ensureWorkflowExists(ctx context.Context, tc *itemporal.Client, traceID string) error {
	if tc == nil {
		return errors.New("Temporal is not configured")
	}
	_, err := tc.SDK().DescribeWorkflowExecution(ctx, agentRunWorkflowID(traceID), "")
	return err
}

func isTemporalNotFound(err error) bool {
	var notFound *serviceerror.NotFound
	return errors.As(err, &notFound)
}

func searchAttributeString(payload *commonpb.Payload) string {
	if payload == nil {
		return ""
	}
	var out string
	if err := converter.GetDefaultDataConverter().FromPayload(payload, &out); err == nil {
		return out
	}
	return ""
}

func searchAttributeInt(payload *commonpb.Payload) int {
	s := searchAttributeString(payload)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
