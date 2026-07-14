// Package modelintelligence is HAI's architecture-aware model layer (§16-§19).
// It makes HAI architecture-aware rather than merely provider-aware: model
// architecture families, routing lanes that affect real behavior, provider
// telemetry, token/context budgets, reasoning-effort control, caching
// boundaries, and a DSpark-compatible inference adapter. It never implements
// foundation-model internals and never fakes provider/model state.
package modelintelligence

import "fmt"

// parseEnum validates v against the allowed set for a ~string enum type.
func parseEnum[T ~string](kind, v string, valid []T) (T, error) {
	for _, x := range valid {
		if string(x) == v {
			return x, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("modelintelligence: invalid %s %q", kind, v)
}

// ArchitectureFamily labels a model's architecture (§16). HAI stores metadata
// only; it does not implement any of these internals.
type ArchitectureFamily string

const (
	ArchDenseTransformer            ArchitectureFamily = "dense_transformer"
	ArchSparseMoETransformer        ArchitectureFamily = "sparse_moe_transformer"
	ArchHighSparsityMoE             ArchitectureFamily = "high_sparsity_moe"
	ArchHybridLinearAttnSparseMoE   ArchitectureFamily = "hybrid_linear_attention_sparse_moe"
	ArchCompressedSparseAttention   ArchitectureFamily = "compressed_sparse_attention"
	ArchHeavilyCompressedAttention  ArchitectureFamily = "heavily_compressed_attention"
	ArchHybridCompressedAttnStack   ArchitectureFamily = "hybrid_compressed_attention_stack"
	ArchGatedDeltanetRecurrentAttn  ArchitectureFamily = "gated_deltanet_recurrent_attention"
	ArchGatedAttentionHybrid        ArchitectureFamily = "gated_attention_hybrid"
	ArchThinkerTalkerOmni           ArchitectureFamily = "thinker_talker_omni"
	ArchAudioTransformerEncoder     ArchitectureFamily = "audio_transformer_encoder"
	ArchMultiCodebookStreamSpeech   ArchitectureFamily = "multi_codebook_streaming_speech"
	ArchAriaTextSpeechAlignment     ArchitectureFamily = "aria_text_speech_alignment"
	ArchDiffusionLanguageModel      ArchitectureFamily = "diffusion_language_model"
	ArchBlockDiffusionLanguageModel ArchitectureFamily = "block_diffusion_language_model"
	ArchTokenizerFreeHierByte       ArchitectureFamily = "tokenizer_free_hierarchical_byte"
	ArchByteEncoderWordBackbone     ArchitectureFamily = "byte_encoder_word_backbone_byte_decoder"
	ArchLoopedRecurrentLM           ArchitectureFamily = "looped_recurrent_language_model"
	ArchDecoupledMoEKnowledge       ArchitectureFamily = "decoupled_moe_knowledge_experts"
	ArchBidirectionalTokenClassify  ArchitectureFamily = "bidirectional_token_classifier"
	ArchSpeculativeDecodingBackend  ArchitectureFamily = "speculative_decoding_backend"
	ArchSpeculativeExpertPrefetch   ArchitectureFamily = "speculative_expert_prefetch_backend"
	ArchHWSWCodesignedInference     ArchitectureFamily = "hardware_software_codesigned_inference"
	ArchWorldModelAgent             ArchitectureFamily = "world_model_agent"
	ArchPlanExecuteVerifyReplan     ArchitectureFamily = "plan_execute_verify_replan_agent"
	ArchReactAgentLoop              ArchitectureFamily = "react_agent_loop"
	ArchTreeSearchAgentLoop         ArchitectureFamily = "tree_search_agent_loop"
	ArchLatsAgentLoop               ArchitectureFamily = "lats_agent_loop"
	ArchMemGPTTieredMemory          ArchitectureFamily = "memgpt_tiered_memory_agent"
	ArchOrchestratorWorkers         ArchitectureFamily = "orchestrator_workers_agent"
	ArchMCPToolRuntime              ArchitectureFamily = "mcp_tool_runtime"
	ArchOpenAICompatibleUnknown     ArchitectureFamily = "openai_compatible_unknown"
	ArchOllamaUnknown               ArchitectureFamily = "ollama_unknown"
	ArchLocalRuntimeUnknown         ArchitectureFamily = "local_runtime_unknown"
	ArchUnknown                     ArchitectureFamily = "unknown"
)

func allArchitectureFamilies() []ArchitectureFamily {
	return []ArchitectureFamily{
		ArchDenseTransformer, ArchSparseMoETransformer, ArchHighSparsityMoE,
		ArchHybridLinearAttnSparseMoE, ArchCompressedSparseAttention, ArchHeavilyCompressedAttention,
		ArchHybridCompressedAttnStack, ArchGatedDeltanetRecurrentAttn, ArchGatedAttentionHybrid,
		ArchThinkerTalkerOmni, ArchAudioTransformerEncoder, ArchMultiCodebookStreamSpeech,
		ArchAriaTextSpeechAlignment, ArchDiffusionLanguageModel, ArchBlockDiffusionLanguageModel,
		ArchTokenizerFreeHierByte, ArchByteEncoderWordBackbone, ArchLoopedRecurrentLM,
		ArchDecoupledMoEKnowledge, ArchBidirectionalTokenClassify, ArchSpeculativeDecodingBackend,
		ArchSpeculativeExpertPrefetch, ArchHWSWCodesignedInference, ArchWorldModelAgent,
		ArchPlanExecuteVerifyReplan, ArchReactAgentLoop, ArchTreeSearchAgentLoop, ArchLatsAgentLoop,
		ArchMemGPTTieredMemory, ArchOrchestratorWorkers, ArchMCPToolRuntime,
		ArchOpenAICompatibleUnknown, ArchOllamaUnknown, ArchLocalRuntimeUnknown, ArchUnknown,
	}
}

func (a ArchitectureFamily) String() string { return string(a) }
func (a ArchitectureFamily) IsValid() bool {
	_, err := parseEnum("architectureFamily", string(a), allArchitectureFamilies())
	return err == nil
}
func ParseArchitectureFamily(v string) (ArchitectureFamily, error) {
	return parseEnum("architectureFamily", v, allArchitectureFamilies())
}

// RoutingLane is a behavioral lane every model call is assigned to (§16). Each
// lane must affect real routing/policy/budget/verification/scheduling output.
type RoutingLane string

const (
	LaneFastTriage          RoutingLane = "fast_triage"
	LaneLongContextDossier  RoutingLane = "long_context_dossier"
	LanePrivacyFilter       RoutingLane = "privacy_filter"
	LaneDrafting            RoutingLane = "drafting"
	LaneParallelBatch       RoutingLane = "parallel_batch"
	LaneByteRobust          RoutingLane = "byte_robust"
	LaneOmniLive            RoutingLane = "omni_live"
	LaneRecursiveDeepReview RoutingLane = "recursive_deep_review"
	LaneVerifier            RoutingLane = "verifier"
)

func allLanes() []RoutingLane {
	return []RoutingLane{
		LaneFastTriage, LaneLongContextDossier, LanePrivacyFilter, LaneDrafting,
		LaneParallelBatch, LaneByteRobust, LaneOmniLive, LaneRecursiveDeepReview, LaneVerifier,
	}
}

func (l RoutingLane) String() string { return string(l) }
func (l RoutingLane) IsValid() bool {
	_, err := parseEnum("routingLane", string(l), allLanes())
	return err == nil
}
func ParseRoutingLane(v string) (RoutingLane, error) {
	return parseEnum("routingLane", v, allLanes())
}

// ProviderStatus is a provider's truthful health (§10.17). A provider is never
// active without a successful probe.
type ProviderStatus string

const (
	ProviderNotConfigured ProviderStatus = "not_configured"
	ProviderConfigured    ProviderStatus = "configured"
	ProviderProbing       ProviderStatus = "probing"
	ProviderActive        ProviderStatus = "active"
	ProviderUnavailable   ProviderStatus = "unavailable"
	ProviderFailed        ProviderStatus = "failed"
	ProviderBlocked       ProviderStatus = "blocked"
)

func allProviderStatuses() []ProviderStatus {
	return []ProviderStatus{
		ProviderNotConfigured, ProviderConfigured, ProviderProbing, ProviderActive,
		ProviderUnavailable, ProviderFailed, ProviderBlocked,
	}
}

func (s ProviderStatus) String() string { return string(s) }
func (s ProviderStatus) IsValid() bool {
	_, err := parseEnum("providerStatus", string(s), allProviderStatuses())
	return err == nil
}

// Usable reports whether a provider may serve a model call (active only).
func (s ProviderStatus) Usable() bool { return s == ProviderActive }
func ParseProviderStatus(v string) (ProviderStatus, error) {
	return parseEnum("providerStatus", v, allProviderStatuses())
}

// ClaimLevel is how strongly a provider/model capability is proven (§10.17).
// Never auto-promote to production_ready.
type ClaimLevel string

const (
	ClaimDocumentedOnly                   ClaimLevel = "documented_only"
	ClaimContractDefined                  ClaimLevel = "contract_defined"
	ClaimConfigured                       ClaimLevel = "configured"
	ClaimProbed                           ClaimLevel = "probed"
	ClaimSmokeTested                      ClaimLevel = "smoke_tested"
	ClaimExercisedLocalSafeTask           ClaimLevel = "exercised_with_local_safe_task"
	ClaimExercisedRealAccountRead         ClaimLevel = "exercised_with_real_account_read"
	ClaimExercisedRealExternalWriteApprov ClaimLevel = "exercised_with_real_external_write_after_approval"
	ClaimBenchmarked                      ClaimLevel = "benchmarked"
	ClaimOperatorVerified                 ClaimLevel = "operator_verified"
	ClaimProductionReady                  ClaimLevel = "production_ready"
)

func allClaimLevels() []ClaimLevel {
	return []ClaimLevel{
		ClaimDocumentedOnly, ClaimContractDefined, ClaimConfigured, ClaimProbed, ClaimSmokeTested,
		ClaimExercisedLocalSafeTask, ClaimExercisedRealAccountRead, ClaimExercisedRealExternalWriteApprov,
		ClaimBenchmarked, ClaimOperatorVerified, ClaimProductionReady,
	}
}

func (c ClaimLevel) String() string { return string(c) }
func (c ClaimLevel) IsValid() bool {
	_, err := parseEnum("claimLevel", string(c), allClaimLevels())
	return err == nil
}
func ParseClaimLevel(v string) (ClaimLevel, error) {
	return parseEnum("claimLevel", v, allClaimLevels())
}

// ReasoningEffort bounds how much reasoning an operation may use (§19).
type ReasoningEffort string

const (
	EffortNone      ReasoningEffort = "none"
	EffortLow       ReasoningEffort = "low"
	EffortMedium    ReasoningEffort = "medium"
	EffortHigh      ReasoningEffort = "high"
	EffortDeep      ReasoningEffort = "deep"
	EffortRecursive ReasoningEffort = "recursive"
)

func allReasoningEfforts() []ReasoningEffort {
	return []ReasoningEffort{EffortNone, EffortLow, EffortMedium, EffortHigh, EffortDeep, EffortRecursive}
}

func (r ReasoningEffort) String() string { return string(r) }
func (r ReasoningEffort) IsValid() bool {
	_, err := parseEnum("reasoningEffort", string(r), allReasoningEfforts())
	return err == nil
}
func ParseReasoningEffort(v string) (ReasoningEffort, error) {
	return parseEnum("reasoningEffort", v, allReasoningEfforts())
}

// ContextStrategy bounds how much context an operation packs (§19). Default is
// minimal/evidence_only; never send everything by default.
type ContextStrategy string

const (
	ContextMinimal         ContextStrategy = "minimal"
	ContextEvidenceOnly    ContextStrategy = "evidence_only"
	ContextSummaryPlusEvid ContextStrategy = "summary_plus_evidence"
	ContextRetrievalTopK   ContextStrategy = "retrieval_top_k"
	ContextLongContextPack ContextStrategy = "long_context_pack"
	ContextDossierMode     ContextStrategy = "dossier_mode"
	ContextHumanSelected   ContextStrategy = "human_selected"
)

func allContextStrategies() []ContextStrategy {
	return []ContextStrategy{
		ContextMinimal, ContextEvidenceOnly, ContextSummaryPlusEvid, ContextRetrievalTopK,
		ContextLongContextPack, ContextDossierMode, ContextHumanSelected,
	}
}

func (c ContextStrategy) String() string { return string(c) }
func (c ContextStrategy) IsValid() bool {
	_, err := parseEnum("contextStrategy", string(c), allContextStrategies())
	return err == nil
}
func ParseContextStrategy(v string) (ContextStrategy, error) {
	return parseEnum("contextStrategy", v, allContextStrategies())
}

// CacheType is a cache layer (§19).
type CacheType string

const (
	CacheDeterministicResult CacheType = "deterministic_result"
	CachePromptPrefix        CacheType = "prompt_prefix"
	CacheSemantic            CacheType = "semantic"
	CacheSourceSummary       CacheType = "source_summary"
	CacheVerification        CacheType = "verification"
)

func allCacheTypes() []CacheType {
	return []CacheType{CacheDeterministicResult, CachePromptPrefix, CacheSemantic, CacheSourceSummary, CacheVerification}
}

func (c CacheType) String() string { return string(c) }
func (c CacheType) IsValid() bool {
	_, err := parseEnum("cacheType", string(c), allCacheTypes())
	return err == nil
}
func ParseCacheType(v string) (CacheType, error) {
	return parseEnum("cacheType", v, allCacheTypes())
}

// Queue is a scheduling queue for model work (§19).
type Queue string

const (
	QueueInteractiveNow   Queue = "interactive_now"
	QueueBackgroundFast   Queue = "background_fast"
	QueueBackgroundBatch  Queue = "background_batch"
	QueueVerifier         Queue = "verifier_queue"
	QueueApproval         Queue = "approval_queue"
	QueueRuntimeExecution Queue = "runtime_execution_queue"
	QueueLowPower         Queue = "low_power_queue"
	QueueNightBatch       Queue = "night_batch"
)

func allQueues() []Queue {
	return []Queue{
		QueueInteractiveNow, QueueBackgroundFast, QueueBackgroundBatch, QueueVerifier,
		QueueApproval, QueueRuntimeExecution, QueueLowPower, QueueNightBatch,
	}
}

func (q Queue) String() string { return string(q) }
func (q Queue) IsValid() bool {
	_, err := parseEnum("queue", string(q), allQueues())
	return err == nil
}
func ParseQueue(v string) (Queue, error) {
	return parseEnum("queue", v, allQueues())
}
