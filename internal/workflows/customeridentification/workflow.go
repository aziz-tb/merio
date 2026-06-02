package customeridentification

import (
	"strconv"
	"time"

	itemporal "github.com/aziz-tb/merio/internal/temporal"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const WorkflowKind = "customer_identification"

type Input = WorkflowInput
type Output = WorkflowOutput

func CustomerIdentificationWorkflow(ctx workflow.Context, input Input) (Output, error) {
	if err := workflow.UpsertSearchAttributes(ctx, map[string]any{
		itemporal.SATraceID:      input.TraceID,
		itemporal.SAWorkflowKind: WorkflowKind,
		itemporal.SAEntityType:   "conversation",
		itemporal.SAEntityID:     strconv.FormatInt(input.ConversationID, 10),
	}); err != nil {
		return Output{}, err
	}

	loadCtx := withRetry(ctx, 2)
	noRetryCtx := withRetry(ctx, 1)

	var op CreateOperationOutput
	if err := workflow.ExecuteActivity(loadCtx, "CreateOperationActivity", CreateOperationInput{
		ConversationID:    input.ConversationID,
		Channel:           input.Channel,
		TriggeredByUserID: input.TriggeredByUserID,
		TraceID:           input.TraceID,
	}).Get(ctx, &op); err != nil {
		return Output{}, err
	}

	var settings Settings
	if err := workflow.ExecuteActivity(loadCtx, "LoadSettingsActivity", LoadSettingsInput{}).Get(ctx, &settings); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}

	var should ShouldRunOutput
	if err := workflow.ExecuteActivity(noRetryCtx, "ShouldRunActivity", ShouldRunInput{
		ConversationID: input.ConversationID,
		Channel:        input.Channel,
		Settings:       settings,
	}).Get(ctx, &should); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}
	if !should.ShouldRun {
		return persistSkipped(ctx, input, op.AIOperationID, should.Reason)
	}

	var prepared PrepareMessageOutput
	if err := workflow.ExecuteActivity(noRetryCtx, "PrepareMessageActivity", PrepareMessageInput{
		ConversationID: input.ConversationID,
		MessageID:      input.MessageID,
		Settings:       settings,
	}).Get(ctx, &prepared); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}

	var pattern PatternDetectOutput
	if err := workflow.ExecuteActivity(noRetryCtx, "PatternDetectActivity", PatternDetectInput{
		PreparedMessage: prepared.PreparedMessage,
		Settings:        settings,
	}).Get(ctx, &pattern); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}

	llmOut := LLMExtractOutput{ExtractedAttrs: map[string]any{}}
	if pattern.ResolvedContractID == nil {
		if err := workflow.ExecuteActivity(loadCtx, "LLMExtractActivity", LLMExtractInput{
			ConversationID:  input.ConversationID,
			TraceID:         input.TraceID,
			Channel:         input.Channel,
			PreparedMessage: prepared.PreparedMessage,
			Settings:        settings,
			PatternAttrs:    pattern.ExtractedAttrs,
		}).Get(ctx, &llmOut); err != nil {
			return persistFailure(ctx, input, op.AIOperationID, err.Error())
		}
	}

	attrs := mergeAttrs(pattern.ExtractedAttrs, llmOut.ExtractedAttrs)
	var search SearchCustomerByFieldsOutput
	if err := workflow.ExecuteActivity(loadCtx, "SearchCustomerByFieldsActivity", SearchCustomerByFieldsInput{
		ConversationID:    input.ConversationID,
		TriggeredByUserID: input.TriggeredByUserID,
		TraceID:           input.TraceID,
		Attrs:             attrs,
	}).Get(ctx, &search); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}

	if search.ContractID == nil && search.CustomerID == nil {
		return persistTerminal(ctx, input, op.AIOperationID, terminalState{
			status:  StatusNoMatch,
			method:  methodFor(pattern, llmOut),
			pattern: pattern,
			llm:     llmOut,
			search:  search,
			attrs:   attrs,
		})
	}

	var loaded LoadContractAndCustomerOutput
	if err := workflow.ExecuteActivity(loadCtx, "LoadContractAndCustomerActivity", LoadContractAndCustomerInput{
		ConversationID:    input.ConversationID,
		TriggeredByUserID: input.TriggeredByUserID,
		TraceID:           input.TraceID,
		ContractID:        search.ContractID,
		CustomerID:        search.CustomerID,
	}).Get(ctx, &loaded); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}
	if loaded.Customer == nil && loaded.Contract == nil {
		return persistTerminal(ctx, input, op.AIOperationID, terminalState{
			status:  StatusNoMatch,
			method:  methodFor(pattern, llmOut),
			pattern: pattern,
			llm:     llmOut,
			search:  search,
			load:    loaded,
			attrs:   attrs,
		})
	}

	var verification EvaluateVerificationOutput
	if err := workflow.ExecuteActivity(noRetryCtx, "EvaluateVerificationActivity", EvaluateVerificationInput{
		Channel:                     input.Channel,
		PreparedMessage:             prepared.PreparedMessage,
		Customer:                    loaded.Customer,
		Contract:                    loaded.Contract,
		ExtractedAttrs:              attrs,
		Rules:                       settings.VerificationRulesAsync,
		CustomerVerification:        search.CustomerVerification,
		CustomerVerificationMessage: search.CustomerVerificationMessage,
	}).Get(ctx, &verification); err != nil {
		return persistFailure(ctx, input, op.AIOperationID, err.Error())
	}

	return persistTerminal(ctx, input, op.AIOperationID, terminalState{
		status:       StatusSuccess,
		method:       methodFor(pattern, llmOut),
		pattern:      pattern,
		llm:          llmOut,
		search:       search,
		load:         loaded,
		verification: verification,
		attrs:        attrs,
	})
}

func withRetry(ctx workflow.Context, attempts int32) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    attempts,
		},
	})
}

type terminalState struct {
	status       string
	method       string
	pattern      PatternDetectOutput
	llm          LLMExtractOutput
	search       SearchCustomerByFieldsOutput
	load         LoadContractAndCustomerOutput
	verification EvaluateVerificationOutput
	attrs        map[string]any
}

func persistTerminal(ctx workflow.Context, input Input, aiOperationID int64, state terminalState) (Output, error) {
	var customerID *string
	var contractID *string
	if state.load.Customer != nil {
		id := state.load.Customer.CustomerID
		customerID = &id
	}
	if state.load.Contract != nil {
		id := state.load.Contract.ContractID
		contractID = &id
	}
	if state.status == StatusNoMatch {
		state.verification = EvaluateVerificationOutput{Level: 0, Source: "none"}
	}
	var persisted PersistResultOutput
	err := workflow.ExecuteActivity(withRetry(ctx, 2), "PersistResultActivity", PersistResultInput{
		AIOperationID:     aiOperationID,
		ConversationID:    input.ConversationID,
		Channel:           input.Channel,
		TriggeredByUserID: input.TriggeredByUserID,
		TraceID:           input.TraceID,
		Status:            state.status,
		Method:            state.method,
		CustomerID:        customerID,
		ContractID:        contractID,
		Verification:      state.verification,
		PatternDetection:  state.pattern,
		LLMExtraction:     state.llm,
		SearchByFields:    state.search,
		Load:              state.load,
	}).Get(ctx, &persisted)
	if err != nil {
		return Output{}, err
	}
	return Output{
		Status:                   state.status,
		CustomerID:               customerID,
		ContractID:               contractID,
		VerificationLevel:        state.verification.Level,
		VerificationMessage:      state.verification.Message,
		AIOperationID:            persisted.AIOperationID,
		PreviousLinkedCustomerID: persisted.PreviousLinkedCustomerID,
	}, nil
}

func persistSkipped(ctx workflow.Context, input Input, aiOperationID int64, reason string) (Output, error) {
	var persisted PersistResultOutput
	err := workflow.ExecuteActivity(withRetry(ctx, 2), "PersistResultActivity", PersistResultInput{
		AIOperationID:     aiOperationID,
		ConversationID:    input.ConversationID,
		Channel:           input.Channel,
		TriggeredByUserID: input.TriggeredByUserID,
		TraceID:           input.TraceID,
		Status:            StatusSkipped,
		Method:            MethodNone,
		SkipReason:        reason,
		Verification:      EvaluateVerificationOutput{Level: 0, Source: "none"},
	}).Get(ctx, &persisted)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: StatusSkipped, AIOperationID: persisted.AIOperationID, PreviousLinkedCustomerID: persisted.PreviousLinkedCustomerID}, nil
}

func persistFailure(ctx workflow.Context, input Input, aiOperationID int64, message string) (Output, error) {
	var persisted PersistResultOutput
	err := workflow.ExecuteActivity(withRetry(ctx, 2), "PersistResultActivity", PersistResultInput{
		AIOperationID:     aiOperationID,
		ConversationID:    input.ConversationID,
		Channel:           input.Channel,
		TriggeredByUserID: input.TriggeredByUserID,
		TraceID:           input.TraceID,
		Status:            StatusFailed,
		Method:            MethodNone,
		ErrorMessage:      message,
		Verification:      EvaluateVerificationOutput{Level: 0, Source: "none"},
	}).Get(ctx, &persisted)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: StatusFailed, AIOperationID: persisted.AIOperationID, PreviousLinkedCustomerID: persisted.PreviousLinkedCustomerID}, nil
}

func mergeAttrs(pattern, extracted map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range pattern {
		out[k] = v
	}
	for k, v := range extracted {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func methodFor(pattern PatternDetectOutput, llmOut LLMExtractOutput) string {
	if pattern.ContractNumberMatch != nil || pattern.CustomerNumberMatch != nil || pattern.SenderEmailMatch != nil || len(pattern.BodyEmailMatches) > 0 {
		return MethodPattern
	}
	if llmOut.Ran && len(llmOut.ExtractedAttrs) > 0 {
		return MethodLLM
	}
	return MethodNone
}
