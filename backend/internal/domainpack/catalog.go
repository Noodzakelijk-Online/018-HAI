package domainpack

type packDefinition struct {
	id           PackID
	name         string
	description  string
	sensitive    bool
	localOnly    bool
	retention    int
	signals      []ClassificationSignal
	entities     []string
	capabilities []string
	intakeFocus  string
}

// BuiltinPacks returns a fresh deep copy of the immutable v1 domain-pack
// catalog. The catalog describes bounded policy and classification behavior;
// it neither installs tools nor grants execution authority.
func BuiltinPacks() []DomainPack {
	definitions := []packDefinition{
		{
			id: PackLegalGovernment, name: "Legal and government", sensitive: true, localOnly: true, retention: 3650,
			description: "Cases, rights, obligations, government correspondence, insurance disputes, hearings, and evidence.",
			signals: []ClassificationSignal{
				strong("lawyer", "professional legal party explicitly named"), strong("court", "court process explicitly named"),
				strong("legal filing", "legal action explicitly requested"), strong("government decision", "government decision explicitly named"),
				moderate("municipality", "government body named"), weak("case", "case is ambiguous without legal context"),
				weak("appeal", "appeal can be non-legal"),
			},
			entities:     []string{"case", "party", "authority", "lawyer", "deadline", "filing", "evidence", "decision"},
			capabilities: []string{"case-management", "evidence-analysis", "legal-drafting", "deadline-tracking"},
			intakeFocus:  "the exact case, authority, deadline, desired remedy, and source records",
		},
		{
			id: PackEmergencyContinuity, name: "Emergency and continuity", sensitive: true, localOnly: true, retention: 2555,
			description: "Immediate danger, critical service disruption, incapacity, recovery, resilience, and continuity planning.",
			signals: []ClassificationSignal{
				strong("immediate danger", "immediate danger explicitly stated"), strong("emergency", "emergency explicitly stated"),
				strong("disaster recovery", "continuity event explicitly stated"), moderate("critical outage", "critical service interruption named"),
				weak("urgent", "urgency alone does not establish an emergency"),
			},
			entities:     []string{"incident", "person-at-risk", "critical-service", "recovery-plan", "emergency-contact", "safe-location"},
			capabilities: []string{"incident-triage", "continuity-planning", "safe-notification-drafting", "recovery-coordination"},
			intakeFocus:  "current danger, affected people, critical services, location, and emergency authority",
		},
		{
			id: PackHealthWellbeing, name: "Health and wellbeing", sensitive: true, localOnly: true, retention: 2555,
			description: "Health records, care plans, symptoms, medication, sleep, stress, treatment, and wellbeing support.",
			signals: []ClassificationSignal{
				strong("medical diagnosis", "diagnosis explicitly named"), strong("doctor", "health professional explicitly named"),
				strong("medication", "medication explicitly named"), moderate("therapy", "care context named"),
				weak("stress", "stress alone is insufficient to infer a health record"), weak("sleep", "sleep can be ordinary planning context"),
			},
			entities:     []string{"person", "clinician", "condition", "symptom", "medication", "appointment", "care-plan", "health-record"},
			capabilities: []string{"health-record-organization", "appointment-planning", "source-grounded-health-research", "care-reminders"},
			intakeFocus:  "the stated health goal, professional guidance, symptoms, medication, consent, and urgent warning signs",
		},
		{
			id: PackFinancial, name: "Financial", sensitive: true, localOnly: true, retention: 3650,
			description: "Budgets, accounts, invoices, taxes, debts, purchases, payments, and financial commitments.",
			signals: []ClassificationSignal{
				strong("bank account", "banking context explicitly named"), strong("pay invoice", "financial transaction explicitly requested"),
				strong("tax return", "tax obligation explicitly named"), moderate("invoice", "financial document named"),
				moderate("debt", "financial liability named"), weak("money", "money alone is too broad"),
			},
			entities:     []string{"account", "transaction", "invoice", "counterparty", "budget", "tax-obligation", "debt", "asset"},
			capabilities: []string{"budget-analysis", "invoice-extraction", "deterministic-calculation", "financial-record-organization"},
			intakeFocus:  "amounts, currency, account, counterparty, obligation, source records, and approval authority",
		},
		{
			id: PackWorkVenture, name: "Work and venture", retention: 2555,
			description: "Employment, clients, services, software, projects, products, businesses, and ventures.",
			signals: []ClassificationSignal{
				strong("client deliverable", "client deliverable explicitly named"), strong("github repository", "software repository explicitly named"),
				moderate("business", "business context named"), moderate("developer", "development role named"),
				moderate("project", "project context named"), weak("work", "work alone is broad"),
			},
			entities:     []string{"organization", "client", "project", "deliverable", "contract", "repository", "milestone", "product"},
			capabilities: []string{"project-planning", "software-review", "client-operations", "venture-analysis"},
			intakeFocus:  "the objective, client or venture, deliverable, acceptance criteria, budget, and deadline",
		},
		{
			id: PackHomeAssets, name: "Home and assets", retention: 2555,
			description: "Housing, garden, maintenance, repairs, warranties, vehicles, contractors, and material assets.",
			signals: []ClassificationSignal{
				strong("home repair", "home repair explicitly named"), strong("property maintenance", "property maintenance explicitly named"),
				moderate("contractor", "contractor context named"), moderate("warranty", "asset warranty named"),
				weak("house", "house can be incidental context"),
			},
			entities:     []string{"property", "room", "asset", "vehicle", "contractor", "repair", "warranty", "maintenance-plan"},
			capabilities: []string{"maintenance-planning", "asset-inventory", "contractor-coordination", "warranty-tracking"},
			intakeFocus:  "the property or asset, condition, urgency, owner, warranty, access, and desired outcome",
		},
		{
			id: PackRelationshipsCare, name: "Relationships and care", sensitive: true, localOnly: true, retention: 1825,
			description: "Relationships, caregiving, interpersonal commitments, support, boundaries, and conflict.",
			signals: []ClassificationSignal{
				strong("caregiver", "care role explicitly named"), strong("relationship conflict", "interpersonal conflict explicitly named"),
				moderate("partner", "close relationship named"), moderate("care plan", "care commitment named"),
				weak("friend", "friend mention alone is insufficient for sensitive classification"),
			},
			entities:     []string{"person", "relationship", "care-duty", "boundary", "commitment", "conversation", "support-plan"},
			capabilities: []string{"care-coordination", "communication-drafting", "commitment-tracking", "conflict-sensitive-planning"},
			intakeFocus:  "the relationship, consent, stated needs, commitments, boundaries, and communication goal",
		},
		{
			id: PackLearningGrowth, name: "Learning and growth", retention: 1825,
			description: "Study, research, courses, skills, practice, competence, and career development.",
			signals: []ClassificationSignal{
				strong("study plan", "study plan explicitly requested"), strong("training course", "course explicitly named"),
				moderate("learn", "learning intent named"), moderate("research topic", "research activity named"),
				weak("skill", "skill can be incidental"),
			},
			entities:     []string{"subject", "course", "skill", "resource", "practice-session", "assessment", "learning-goal"},
			capabilities: []string{"curriculum-planning", "research", "practice-scheduling", "competence-assessment"},
			intakeFocus:  "the learning outcome, current level, source quality, available time, and assessment method",
		},
		{
			id: PackTravelMobility, name: "Travel and mobility", retention: 730,
			description: "Trips, routes, transport, accommodation, visas, accessibility, and mobility.",
			signals: []ClassificationSignal{
				strong("flight booking", "flight transaction explicitly named"), strong("travel itinerary", "itinerary explicitly named"),
				moderate("train", "transport mode named"), moderate("hotel", "accommodation named"), weak("route", "route alone is broad"),
			},
			entities:     []string{"trip", "traveller", "route", "booking", "ticket", "accommodation", "visa", "vehicle"},
			capabilities: []string{"itinerary-planning", "availability-checking", "travel-research", "booking-draft"},
			intakeFocus:  "travellers, dates, origin, destination, budget, accessibility, documents, and booking authority",
		},
		{
			id: PackPersonalProductivity, name: "Personal productivity", retention: 730,
			description: "Tasks, focus, routines, habits, reviews, scheduling, and personal execution.",
			signals: []ClassificationSignal{
				strong("weekly review", "review workflow explicitly named"), strong("task list", "task system explicitly named"),
				moderate("focus", "attention goal named"), moderate("routine", "routine named"), weak("todo", "todo can be incidental"),
			},
			entities:     []string{"task", "routine", "habit", "time-block", "review", "goal", "distraction"},
			capabilities: []string{"task-planning", "calendar-planning", "habit-support", "review-summarization"},
			intakeFocus:  "the desired outcome, available capacity, due date, dependencies, and review cadence",
		},
		{
			id: PackFamilyHousehold, name: "Family and household", sensitive: true, localOnly: true, retention: 1825,
			description: "Family responsibilities, household schedules, shared decisions, dependants, and domestic coordination.",
			signals: []ClassificationSignal{
				strong("family schedule", "family schedule explicitly named"), strong("household responsibility", "household duty explicitly named"),
				moderate("childcare", "dependant care named"), weak("family", "family mention alone is insufficient for sensitive classification"),
			},
			entities:     []string{"household", "family-member", "dependant", "responsibility", "shared-calendar", "appointment", "agreement"},
			capabilities: []string{"household-coordination", "shared-scheduling", "care-reminders", "responsibility-tracking"},
			intakeFocus:  "the household members, consent, responsibilities, schedule, dependencies, and decision owner",
		},
		{
			id: PackFoodNutrition, name: "Food and nutrition", sensitive: true, localOnly: true, retention: 730,
			description: "Meals, groceries, cooking, dietary preferences, nutrition, allergies, and food planning.",
			signals: []ClassificationSignal{
				strong("food allergy", "allergy explicitly named"), strong("nutrition plan", "nutrition goal explicitly named"),
				moderate("meal plan", "meal planning requested"), moderate("groceries", "food purchasing context named"),
				weak("diet", "diet can be non-medical and ambiguous"),
			},
			entities:     []string{"meal", "ingredient", "allergy", "preference", "recipe", "grocery-list", "nutrition-goal"},
			capabilities: []string{"meal-planning", "allergen-checking", "grocery-planning", "nutrition-source-review"},
			intakeFocus:  "people, allergies, dietary preferences, health guidance, budget, schedule, and available ingredients",
		},
		{
			id: PackCommunication, name: "Communication and correspondence", retention: 1825,
			description: "Email, letters, messages, replies, inboxes, recipients, tone, and correspondence workflows.",
			signals: []ClassificationSignal{
				strong("draft email", "email drafting explicitly requested"), strong("send message", "external communication explicitly requested"),
				moderate("reply", "reply action named"), moderate("letter", "written correspondence named"),
				weak("message", "message can be incidental"),
			},
			entities:     []string{"message", "thread", "sender", "recipient", "attachment", "draft", "commitment", "follow-up"},
			capabilities: []string{"message-drafting", "thread-summarization", "commitment-extraction", "follow-up-planning"},
			intakeFocus:  "the recipient, purpose, source thread, tone, attachments, deadline, and send authority",
		},
		{
			id: PackDigitalAccounts, name: "Digital accounts", sensitive: true, localOnly: true, retention: 1825,
			description: "Online accounts, identity, credentials, access, OAuth, subscriptions, recovery, and cloud services.",
			signals: []ClassificationSignal{
				strong("password reset", "credential recovery explicitly named"), strong("oauth access", "delegated account access explicitly named"),
				strong("account deletion", "destructive account action explicitly named"), moderate("subscription", "digital service commitment named"),
				weak("login", "login alone does not justify sensitive account inference"),
			},
			entities:     []string{"account", "provider", "identity", "permission", "subscription", "recovery-method", "session"},
			capabilities: []string{"account-inventory", "permission-review", "recovery-planning", "subscription-tracking"},
			intakeFocus:  "the provider, account owner, requested access, permissions, recovery route, and verified identity",
		},
		{
			id: PackPossessionsInventory, name: "Possessions and inventory", retention: 2555,
			description: "Possessions, equipment, storage, serial numbers, condition, location, and inventory control.",
			signals: []ClassificationSignal{
				strong("asset inventory", "inventory explicitly requested"), strong("serial number", "inventory identifier named"),
				moderate("equipment", "equipment context named"), moderate("storage box", "storage location named"),
				weak("item", "item is too broad"),
			},
			entities:     []string{"item", "category", "serial-number", "location", "condition", "owner", "receipt", "warranty"},
			capabilities: []string{"inventory-management", "duplicate-detection", "document-linking", "condition-tracking"},
			intakeFocus:  "the item, owner, identifier, location, condition, value, and supporting record",
		},
		{
			id: PackAnimalsDependants, name: "Animals and dependants", sensitive: true, localOnly: true, retention: 1825,
			description: "Animal care, dependant routines, appointments, welfare, supplies, and delegated care.",
			signals: []ClassificationSignal{
				strong("veterinarian appointment", "animal health appointment explicitly named"), strong("dependant care", "dependant duty explicitly named"),
				moderate("pet care", "animal care context named"), weak("dog", "animal mention alone is insufficient for sensitive classification"),
			},
			entities:     []string{"animal", "dependant", "caregiver", "appointment", "medication", "routine", "supply", "welfare-signal"},
			capabilities: []string{"care-scheduling", "welfare-reminders", "record-organization", "supply-planning"},
			intakeFocus:  "the dependant, caregiver authority, welfare needs, professional guidance, schedule, and emergency contact",
		},
		{
			id: PackCommunityCivic, name: "Community and civic", retention: 1825,
			description: "Neighbourhoods, volunteering, civic participation, consultations, elections, and community projects.",
			signals: []ClassificationSignal{
				strong("public consultation", "formal civic process explicitly named"), strong("volunteer project", "community work explicitly named"),
				moderate("neighbourhood", "community context named"), moderate("election", "civic event named"),
				weak("community", "community is broad"),
			},
			entities:     []string{"community", "organization", "initiative", "event", "stakeholder", "public-record", "commitment"},
			capabilities: []string{"community-research", "stakeholder-mapping", "event-planning", "public-draft-review"},
			intakeFocus:  "the community, public process, stakeholders, evidence, desired outcome, and representation authority",
		},
		{
			id: PackLeisure, name: "Leisure", retention: 365,
			description: "Recreation, hobbies, social activities, rest, entertainment, and recovery time.",
			signals: []ClassificationSignal{
				strong("leisure plan", "leisure planning explicitly requested"), strong("hobby project", "hobby project explicitly named"),
				moderate("recreation", "recreation context named"), moderate("game night", "activity named"),
				weak("fun", "fun is too broad"),
			},
			entities:     []string{"activity", "participant", "venue", "booking", "equipment", "preference", "recovery-time"},
			capabilities: []string{"activity-research", "schedule-planning", "preference-matching", "booking-draft"},
			intakeFocus:  "the participants, interests, time, budget, location, accessibility, and desired experience",
		},
		{
			id: PackCreativity, name: "Creativity", retention: 1825,
			description: "Writing, art, music, photography, design, creative projects, and expression.",
			signals: []ClassificationSignal{
				strong("creative project", "creative project explicitly named"), strong("write a story", "creative writing explicitly requested"),
				moderate("photography", "creative medium named"), moderate("music", "creative medium named"),
				weak("design", "design can refer to technical work"),
			},
			entities:     []string{"work", "medium", "draft", "reference", "audience", "rights", "milestone"},
			capabilities: []string{"creative-planning", "drafting", "reference-organization", "rights-aware-review"},
			intakeFocus:  "the creative intent, medium, audience, references, constraints, rights, and completion standard",
		},
		{
			id: PackMeaningValues, name: "Meaning and values", sensitive: true, localOnly: true, retention: 3650,
			description: "Values, purpose, spirituality, identity-consistent choices, meaning, and reflection.",
			signals: []ClassificationSignal{
				strong("values reflection", "values work explicitly requested"), strong("purpose in life", "meaning question explicitly stated"),
				moderate("spiritual practice", "spiritual context explicitly named"), weak("meaning", "meaning alone is ambiguous"),
			},
			entities:     []string{"value", "principle", "role", "commitment", "reflection", "practice", "life-goal"},
			capabilities: []string{"reflective-questioning", "values-mapping", "decision-tradeoff", "goal-alignment"},
			intakeFocus:  "the operator-stated values, question, context, uncertainty, boundaries, and desired reflection outcome",
		},
		{
			id: PackEnvironmentSustainability, name: "Environment and sustainability", retention: 1825,
			description: "Energy, waste, biodiversity, environmental impact, resilience, and sustainable choices.",
			signals: []ClassificationSignal{
				strong("energy consumption", "measurable energy context named"), strong("sustainability plan", "sustainability work explicitly requested"),
				moderate("recycling", "waste practice named"), moderate("carbon footprint", "environmental measure named"),
				weak("green", "green is ambiguous"),
			},
			entities:     []string{"resource", "consumption", "waste-stream", "measure", "habitat", "supplier", "target"},
			capabilities: []string{"impact-calculation", "source-grounded-research", "resource-monitoring", "improvement-planning"},
			intakeFocus:  "the scope, baseline, measurement source, target, constraints, and possible trade-offs",
		},
		{
			id: PackLegacyLongTerm, name: "Legacy and long-term", sensitive: true, localOnly: true, retention: 3650,
			description: "Long-term stewardship, succession, archives, estate context, continuity, and future generations.",
			signals: []ClassificationSignal{
				strong("estate plan", "estate context explicitly named"), strong("succession plan", "succession explicitly named"),
				strong("long-term archive", "long-term record stewardship named"), moderate("future generations", "legacy horizon named"),
				weak("legacy", "legacy alone can be a software term"),
			},
			entities:     []string{"asset", "beneficiary", "steward", "archive", "instruction", "milestone", "contingency"},
			capabilities: []string{"long-term-planning", "archive-organization", "succession-drafting", "continuity-analysis"},
			intakeFocus:  "the long-term intent, people, assets, authority, jurisdiction, records, and review cadence",
		},
		{
			id: PackSafetySecurity, name: "Safety and security", sensitive: true, localOnly: true, retention: 2555,
			description: "Personal safety, security incidents, threats, protective measures, access, and risk reduction.",
			signals: []ClassificationSignal{
				strong("security incident", "security incident explicitly named"), strong("personal safety threat", "personal threat explicitly named"),
				strong("burglary", "physical security event named"), moderate("unsafe location", "safety condition named"),
				weak("security", "security alone can be technical or generic"),
			},
			entities:     []string{"incident", "person", "location", "asset", "threat", "control", "evidence", "response-plan"},
			capabilities: []string{"risk-assessment", "incident-documentation", "protective-planning", "evidence-preservation"},
			intakeFocus:  "the immediate risk, affected people, location, evidence, existing controls, and emergency authority",
		},
	}

	packs := make([]DomainPack, 0, len(definitions))
	for _, definition := range definitions {
		packs = append(packs, buildPack(definition))
	}
	return clonePacks(packs)
}

func buildPack(def packDefinition) DomainPack {
	pack := DomainPack{
		ID:                    def.id,
		Version:               CatalogVersion,
		Name:                  def.name,
		Description:           def.description,
		Sensitive:             def.sensitive,
		DefaultEnabled:        true,
		ClassificationSignals: cloneSignals(def.signals),
		IntakeQuestions: []IntakeQuestion{
			{ID: "objective", Question: "What outcome should this work achieve?", Required: true},
			{ID: "scope", Question: "What is in scope and what must remain untouched?", Required: true},
			{ID: "domain_context", Question: "Provide " + def.intakeFocus + ".", Required: true},
			{ID: "sources", Question: "Which original sources support the request?", Required: def.sensitive},
			{ID: "authority", Question: "Who may approve or execute consequential actions?", Required: true},
		},
		CommonEntities: cloneStrings(def.entities),
		RiskTriggers: []RiskTrigger{
			{ID: "external_effect", Signal: "external communication or state change", Level: RiskHigh, Explanation: "External effects can create obligations or reputational harm."},
			{ID: "irreversible", Signal: "irreversible or destructive operation", Level: RiskCritical, Explanation: "Irreversible work requires human review and rollback planning."},
			{ID: "uncertain_evidence", Signal: "missing, stale, conflicting, or unsupported evidence", Level: RiskHigh, Explanation: "Consequential work cannot rely on unsupported model output."},
		},
		ApprovalRules: []ApprovalRule{
			{Action: "paid_model_usage", Required: true, MinimumRisk: RiskHigh, Reason: "Paid provider use requires explicit budget approval."},
			{Action: "external_send", Required: true, MinimumRisk: RiskHigh, Reason: "External communication requires scoped human approval."},
			{Action: "public_post", Required: true, MinimumRisk: RiskHigh, Reason: "Publication has reputational and legal consequences."},
			{Action: "financial_transaction", Required: true, MinimumRisk: RiskCritical, Reason: "Moving or committing money requires explicit approval."},
			{Action: "legal_or_government_action", Required: true, MinimumRisk: RiskCritical, Reason: "Legal and government actions require explicit informed approval."},
			{Action: "medical_action", Required: true, MinimumRisk: RiskCritical, Reason: "Medical decisions require qualified human judgment."},
			{Action: "destructive_change", Required: true, MinimumRisk: RiskCritical, Reason: "Destructive changes require confirmation and a recovery plan."},
			{Action: "account_change", Required: true, MinimumRisk: RiskHigh, Reason: "Account and permission changes require verified owner approval."},
		},
		ProhibitedAutonomousActions: []ProhibitedAction{
			{Action: "spend_or_transfer_money", Reason: "The agent may prepare but never independently commit funds."},
			{Action: "make_legal_filing_or_concession", Reason: "The agent may draft but not create legal consequences autonomously."},
			{Action: "diagnose_prescribe_or_change_treatment", Reason: "The agent is not a qualified medical decision-maker."},
			{Action: "publish_or_send_as_owner", Reason: "The owner must approve consequential external representation."},
			{Action: "permanently_delete_or_revoke_access", Reason: "Irreversible deletion and access revocation require human confirmation."},
		},
		SourceAuthorityRules: []SourceAuthorityRule{
			{ClaimType: "operator_specific_fact", AcceptedSources: []string{"owner-confirmed record", "connected source record", "primary document"}, MinimumSources: 1, Reason: "Personal facts must come from the owner or a linked source."},
			{ClaimType: "current_public_fact", AcceptedSources: []string{"official source", "primary source"}, MinimumSources: 1, Reason: "Current facts require fresh authoritative evidence."},
			{ClaimType: "consequential_disputed_fact", AcceptedSources: []string{"primary document", "official record", "independent corroborating source"}, MinimumSources: 2, Reason: "Disputed consequential claims need corroboration or review."},
		},
		EvidenceRequirements: []EvidenceRequirement{
			{ID: "source_provenance", Description: "Retain source identity, retrieval time, scope, and evidence link.", RequiredForActions: []string{"store_fact", "external_send", "public_post"}, MinimumVerification: "source_supported"},
			{ID: "execution_result", Description: "Capture deterministic result or authoritative response before completion.", RequiredForActions: []string{"state_change", "complete_task"}, MinimumVerification: "verified"},
			{ID: "approval_record", Description: "Retain approver, scope, decision, and expiry for gated work.", RequiredForActions: []string{"high_risk_action"}, MinimumVerification: "human_approved"},
		},
		DeterministicValidators: []DeterministicValidator{
			{ID: "required_fields", Kind: "schema", Description: "Validate required fields, enum values, identifiers, and bounded lengths."},
			{ID: "deadline_order", Kind: "temporal", Description: "Validate dates, time zones, expiry, and chronological ordering."},
			{ID: "numeric_recalculation", Kind: "calculation", Description: "Recalculate consequential numbers without relying on model arithmetic."},
			{ID: "source_reachability", Kind: "provenance", Description: "Validate that cited source references resolve to permitted records."},
			{ID: "policy_gate", Kind: "policy", Description: "Validate risk, authority, approval scope, and prohibited-action rules."},
		},
		SuccessCriteriaTemplates: []SuccessCriteriaTemplate{
			{ID: "prepared", Criteria: []string{"requested deliverable exists", "required sources are linked", "uncertainties are explicit", "next owner is named"}},
			{ID: "executed", Criteria: []string{"execution stayed within approved scope", "external result is captured", "deterministic validation passed", "audit event is recorded"}},
			{ID: "completed", Criteria: []string{"outcome matches the stated objective", "no blocking open loop is hidden", "verification status is sufficient", "retention policy is applied"}},
		},
		StopEscalationConditions: []StopCondition{
			{ID: "authority_missing", Condition: "required authority or approval is absent, expired, or out of scope", EscalateTo: "owner_approval", Level: RiskHigh},
			{ID: "evidence_conflict", Condition: "authoritative sources conflict on a consequential fact", EscalateTo: "human_review", Level: RiskHigh},
			{ID: "safety_risk", Condition: "credible immediate safety risk or emergency is detected", EscalateTo: "emergency_protocol", Level: RiskCritical},
			{ID: "scope_expansion", Condition: "execution would exceed requested action, tool, resource, or owner scope", EscalateTo: "replan_and_approve", Level: RiskHigh},
			{ID: "repeated_failure", Condition: "bounded retries are exhausted or verification repeatedly fails", EscalateTo: "operator_review", Level: RiskMedium},
		},
		Retention: RetentionPolicy{
			DefaultDays:       def.retention,
			LocalOnly:         def.localOnly || def.sensitive,
			DeletionReview:    def.sensitive,
			ArchiveProvenance: true,
		},
		SuitableAgentCapabilities: cloneStrings(def.capabilities),
		AuditEvents: []string{
			"domain_pack_classified", "domain_pack_selected", "domain_pack_suppressed",
			"risk_triggered", "approval_required", "approval_resolved", "execution_blocked",
			"evidence_attached", "validation_completed", "outcome_verified", "retention_applied",
		},
	}
	return pack
}

func weak(phrase, reason string) ClassificationSignal {
	return ClassificationSignal{Phrase: phrase, Strength: SignalWeak, Reason: reason}
}

func moderate(phrase, reason string) ClassificationSignal {
	return ClassificationSignal{Phrase: phrase, Strength: SignalModerate, Reason: reason}
}

func strong(phrase, reason string) ClassificationSignal {
	return ClassificationSignal{Phrase: phrase, Strength: SignalStrong, Reason: reason}
}
