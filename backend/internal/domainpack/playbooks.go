package domainpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const (
	wholeLifeSpecificationTitle = "HAI Framework of Frameworks"
	wholeLifeSpecificationRef   = "hai-spec://framework-of-frameworks"
)

type methodGroupDefinition struct {
	packID                PackID
	group                 string
	domain                string
	section               string
	purpose               string
	trigger               string
	requiredInputs        []string
	producedOutputs       []string
	authorityRequirements []string
	safetyInvariants      []string
	riskCeiling           RiskLevel
	evidenceRequirements  []string
	evaluationMethod      string
	evaluationCriteria    []string
	methodNames           []string
}

// wholeLifeMethodGroups is deliberately catalog data rather than executable
// behavior. The stable work_venture pack contains two separately identifiable
// groups to preserve existing pack IDs and owner preferences.
var wholeLifeMethodGroups = []methodGroupDefinition{
	{
		packID: PackHealthWellbeing, group: "health_personal_care", domain: "Health and personal care", section: "40",
		purpose: "support safe, source-grounded health documentation, self-management planning, and professional care coordination",
		trigger: "A health, symptom, medication, care, accessibility, or wellbeing task needs structured analysis.",
		requiredInputs: []string{
			"owner-confirmed health objective and consent",
			"current symptoms, observations, medication list, or care records as applicable",
			"qualified professional guidance and emergency context where applicable",
		},
		producedOutputs: []string{
			"source-linked health support record or care-planning artifact",
			"explicit uncertainties, red flags, escalation threshold, and responsible human",
		},
		authorityRequirements: []string{
			"the owner or authorized caregiver controls personal health data",
			"a qualified clinician decides diagnosis, prescription, treatment, and urgent clinical action",
		},
		safetyInvariants: []string{
			"never diagnose, prescribe, change treatment, or silently replace a qualified medical professional",
			"credible urgent red flags stop ordinary planning and trigger human or emergency escalation",
			"health data remains local-only by default and is disclosed only with scoped consent",
		},
		riskCeiling: RiskCritical,
		evidenceRequirements: []string{
			"owner-confirmed observations are distinguished from clinical findings",
			"medication and treatment facts cite a current professional or authoritative source",
			"care decisions retain who advised, who approved, and when",
		},
		evaluationMethod: "clinical-support safety review",
		evaluationCriteria: []string{
			"the artifact separates observation, interpretation, and professional decision",
			"red flags and escalation routes are explicit",
			"no unsupported diagnosis or treatment instruction is presented as fact",
		},
		methodNames: []string{
			"Biopsychosocial assessment", "SOAP", "SBAR", "PQRST symptom description",
			"Red-flag triage", "Shared decision-making", "Medication reconciliation",
			"Medication-rights checklist", "Care-plan cycles", "ADL/IADL monitoring",
			"Sleep hygiene", "Pacing", "Energy envelope", "Pain diary", "Trigger tracking",
			"Symptom trend analysis", "Preventive-care schedules", "Escalation thresholds",
			"Emergency contacts", "Caregiver coordination", "Accessibility accommodations",
		},
	},
	{
		packID: PackFinancial, group: "financial_management", domain: "Financial management", section: "41",
		purpose: "support deterministic financial planning, control, forecasting, and record stewardship without moving money",
		trigger: "A budget, cash-flow, debt, asset, tax, payment-control, or financial-record task needs a structured method.",
		requiredInputs: []string{
			"verified owner scope, currency, period, and financial objective",
			"source-backed balances, transactions, obligations, rates, and assumptions as applicable",
			"applicable budget, mandate, tax calendar, and counterparty records",
		},
		producedOutputs: []string{
			"reconciled calculation, forecast, comparison, control record, or decision scenario",
			"assumptions, sensitivities, approval gates, and unresolved financial risks",
		},
		authorityRequirements: []string{
			"the verified account owner controls financial records and mandates",
			"every payment, transfer, commitment, tax filing, or account change requires scoped human approval",
		},
		safetyInvariants: []string{
			"selection never authorizes a payment, transfer, purchase, filing, or financial commitment",
			"consequential arithmetic is recalculated deterministically from cited inputs",
			"fraud signals, mismatches, and mandate breaches stop the workflow for human review",
		},
		riskCeiling: RiskCritical,
		evidenceRequirements: []string{
			"amounts and balances link to current statements, invoices, contracts, or owner-confirmed records",
			"rates, taxes, and external rules cite fresh authoritative sources",
			"scenario assumptions and uncertainty ranges remain visible",
		},
		evaluationMethod: "deterministic financial reconciliation",
		evaluationCriteria: []string{
			"inputs reconcile to source totals",
			"calculations reproduce exactly",
			"no transaction or commitment is marked approved without a matching human authorization",
		},
		methodNames: []string{
			"Zero-based budgeting", "Envelope budgeting", "50/30/20", "Pay-yourself-first",
			"Cash-flow forecasting", "Sinking funds", "Emergency-fund ladder", "Debt snowball",
			"Debt avalanche", "Net-worth tracking", "Asset-liability management",
			"Liquidity management", "Expected-value decisions", "Scenario analysis",
			"Sensitivity analysis", "Total Cost of Ownership", "Life-cycle costing",
			"Opportunity cost", "Risk-adjusted return", "Fraud controls", "Four-eyes payments",
			"Spending mandates", "Subscription review", "Tax-calendar management",
			"Financial document retention",
		},
	},
	{
		packID: PackHomeAssets, group: "home_garden_assets", domain: "Home, garden and asset management", section: "42",
		purpose: "organize assets, maintenance, inventory, resources, contractors, and garden care with traceable work controls",
		trigger: "A property, garden, inventory, maintenance, utility, contractor, warranty, or asset task needs structured management.",
		requiredInputs: []string{
			"identified property, asset, plant, stock item, or work area",
			"condition, location, ownership, warranty, maintenance, inspection, and usage records as applicable",
			"budget, access, environmental constraints, quotations, and safety requirements",
		},
		producedOutputs: []string{
			"asset, maintenance, inventory, inspection, or work-order artifact",
			"prioritized intervention with cost, safety, timing, and provenance",
		},
		authorityRequirements: []string{
			"the owner or delegated property custodian approves access and consequential work",
			"licensed or qualified professionals perform regulated, hazardous, or specialist work",
		},
		safetyInvariants: []string{
			"never authorize hazardous work, contractor acceptance, purchase, disposal, or access without required human approval",
			"do not delete asset, warranty, inspection, or evidence records automatically",
			"environmental and pest-control recommendations prioritize lawful least-harm controls",
		},
		riskCeiling: RiskHigh,
		evidenceRequirements: []string{
			"asset identity, condition, and work history link to inspection, photo, receipt, manual, or owner record",
			"contractor and quotation comparisons preserve scope and exclusions",
			"predictive claims identify the measurement and confidence basis",
		},
		evaluationMethod: "asset and work-control verification",
		evaluationCriteria: []string{
			"the correct asset and current condition are identified",
			"the recommendation observes safety, warranty, environmental, and approval constraints",
			"completion requires an inspection or source-backed result",
		},
		methodNames: []string{
			"Asset registry", "Preventive maintenance", "Predictive maintenance",
			"Reliability-Centred Maintenance", "FMEA", "Seasonal maintenance", "5S",
			"Visual management", "ABC inventory", "XYZ inventory", "Reorder points",
			"Min-max stocking", "Bill of materials", "Tool tracking", "Warranty tracking",
			"Inspection checklists", "Energy monitoring", "Water monitoring", "Waste hierarchy",
			"Circular-economy principles", "Integrated Pest Management",
			"Plant-health monitoring", "Weather-aware planning", "Work-order management",
			"Contractor and quotation comparison",
		},
	},
	{
		packID: PackWorkVenture, group: "work_service_delivery", domain: "Work and service delivery", section: "43",
		purpose: "turn service and project commitments into safe, measurable, accepted, and continuously improved delivery",
		trigger: "A client, project, service, process, capacity, quality, safety, scope, cost, or delivery task needs a working method.",
		requiredInputs: []string{
			"defined client or internal objective, scope, deliverable, owner, and acceptance criteria",
			"dependencies, skills, capacity, cost, schedule, quality, and safety constraints",
			"source contract, request, process record, or project evidence",
		},
		producedOutputs: []string{
			"traceable plan, process, schedule, responsibility map, quality gate, or service record",
			"verified deliverable state, variances, decisions, and lessons learned",
		},
		authorityRequirements: []string{
			"the accountable owner approves commitments, scope changes, client communication, and acceptance",
			"qualified personnel control safety-critical or regulated service work",
		},
		safetyInvariants: []string{
			"never represent work as accepted or complete without source-backed acceptance evidence",
			"scope, cost, safety, and external commitments cannot expand silently",
			"client sends, invoices, destructive changes, and consequential execution remain approval-gated",
		},
		riskCeiling: RiskHigh,
		evidenceRequirements: []string{
			"requirements and acceptance criteria link to the originating client or project source",
			"time, material, cost, capacity, and quality claims use auditable records",
			"completion captures validation result and accountable acceptance",
		},
		evaluationMethod: "delivery acceptance and quality-gate review",
		evaluationCriteria: []string{
			"the output matches documented scope and acceptance criteria",
			"safety and quality gates pass",
			"variances, open loops, and responsible owners remain visible",
		},
		methodNames: []string{
			"Standard operating procedures", "Checklists", "Job breakdown", "Work packages",
			"Critical Path", "PERT", "Lean", "5S", "Kaizen", "Theory of Constraints",
			"Value-stream mapping", "DMAIC", "Root-cause analysis", "Quality gates",
			"Safety briefings", "Toolbox talks", "Resource planning", "Capacity planning",
			"Skills matrices", "RACI", "Service blueprints", "Customer journey mapping",
			"CRM pipelines", "Quote-accept-deliver-invoice-follow-up", "Scope-change control",
			"Time-and-material tracking", "Cost estimation", "Lessons learned",
		},
	},
	{
		packID: PackWorkVenture, group: "entrepreneurship_venture", domain: "Entrepreneurship and venture", section: "44",
		purpose: "frame, test, compare, govern, and scale venture hypotheses using evidence rather than unsupported confidence",
		trigger: "A venture, market, customer, product, strategy, portfolio, metric, or business-model decision needs a structured method.",
		requiredInputs: []string{
			"defined venture objective, decision horizon, constraints, and accountable owner",
			"customer, market, competitor, product, financial, and operational evidence as applicable",
			"explicit hypotheses, assumptions, success measures, and stop criteria",
		},
		producedOutputs: []string{
			"testable strategy, model, experiment, portfolio, metric, or decision artifact",
			"evidence map distinguishing validated learning, assumptions, uncertainty, and next test",
		},
		authorityRequirements: []string{
			"the venture owner approves strategy, spending, commitments, publication, and market-facing action",
			"affected stakeholders approve use of confidential, personal, or proprietary evidence",
		},
		safetyInvariants: []string{
			"framework selection does not validate a business hypothesis or grant execution authority",
			"market size, product-market fit, and financial claims remain labelled as evidence, inference, or assumption",
			"customer contact, spend, contracts, hiring, and public claims remain approval-gated",
		},
		riskCeiling: RiskHigh,
		evidenceRequirements: []string{
			"customer and market conclusions cite primary research or authoritative data",
			"financial and growth metrics provide definitions, periods, denominators, and exclusions",
			"experiments record hypothesis, intervention, result, and decision",
		},
		evaluationMethod: "hypothesis and decision-quality review",
		evaluationCriteria: []string{
			"the selected method fits the venture decision",
			"claims are traceable to evidence or marked as assumptions",
			"next action has an owner, metric, budget gate, and stopping rule",
		},
		methodNames: []string{
			"Business Model Canvas", "Lean Canvas", "Value Proposition Canvas",
			"Jobs to Be Done", "Lean Startup", "Build-Measure-Learn", "Customer Development",
			"Design Thinking", "Double Diamond", "Effectuation",
			"Causation versus effectuation", "Blue Ocean Strategy", "Strategy Canvas",
			"SWOT", "TOWS", "PESTEL", "Porter's Five Forces", "VRIO", "Ansoff Matrix",
			"BCG Matrix", "GE-McKinsey Matrix", "Three Horizons", "Wardley Mapping",
			"Crossing the Chasm", "Technology Adoption Lifecycle", "Stage-Gate",
			"Product-market fit", "North Star Metric", "AARRR", "Unit economics",
			"Cohort analysis", "TAM/SAM/SOM", "Scenario planning", "Theory of Change",
			"Stakeholder mapping", "OKRs", "Balanced Scorecard",
		},
	},
	{
		packID: PackLegalGovernment, group: "legal_government_case", domain: "Legal, government and case management", section: "45",
		purpose: "organize legal and administrative matters into source-grounded issues, authorities, evidence, deadlines, remedies, and reviewable drafts",
		trigger: "A legal, government, complaint, objection, appeal, information-rights, privacy, evidence, or case task needs a structured method.",
		requiredInputs: []string{
			"identified jurisdiction, authority, case, parties, objective, and procedural posture",
			"primary documents, correspondence, decisions, evidence, deadlines, and owner instructions",
			"qualified legal advice where the task can create legal consequences",
		},
		producedOutputs: []string{
			"source-linked case analysis, chronology, evidence matrix, deadline record, draft, or remedy map",
			"explicit authority hierarchy, uncertainty, conflicts, approval gate, and next procedural owner",
		},
		authorityRequirements: []string{
			"the case owner controls instructions, personal data, and final decisions",
			"a qualified lawyer or authorized representative decides legal advice, filing, concession, settlement, or representation",
		},
		safetyInvariants: []string{
			"never file, concede, settle, waive rights, contact authorities, or represent the owner autonomously",
			"every consequential factual or legal claim must link to suitable primary authority or evidence",
			"deadlines, limitation periods, and jurisdiction remain unverified until confirmed from an authoritative source",
			"legal evidence is retained with provenance and never automatically deleted",
		},
		riskCeiling: RiskCritical,
		evidenceRequirements: []string{
			"legal propositions cite current primary authority for the jurisdiction",
			"case facts link to original records and distinguish allegation, evidence, decision, and inference",
			"evidence handling preserves authenticity and chain of custody",
		},
		evaluationMethod: "legal-source and procedural review",
		evaluationCriteria: []string{
			"issues, rules, facts, analysis, and requested remedy are distinguishable",
			"citations support their attached claims",
			"deadlines, authority, conflicts, and approval status are explicit",
		},
		methodNames: []string{
			"IRAC", "CRAC", "CREAC", "FIRAC", "Claim-issue-evidence matrices",
			"Chronology", "Procedural timeline", "Deadline and limitation tracking",
			"Burden-of-proof mapping", "Elements-of-claim analysis", "Authority hierarchy",
			"Primary-source verification", "Evidence chain of custody", "Document authenticity",
			"Contradiction matrix", "Damage schedules", "Remedy mapping", "Stakeholder mapping",
			"Escalation ladders", "Complaint-objection-appeal workflows",
			"Freedom-of-information workflows", "GDPR access and correction workflows",
			"Legal-hold management", "Correspondence registers", "Promise and commitment tracking",
		},
	},
	{
		packID: PackCommunication, group: "communication", domain: "Communication", section: "46",
		purpose: "prepare clear, audience-aware, source-grounded, and approval-controlled communication without impersonating the owner",
		trigger: "A message, email, letter, presentation, negotiation, difficult conversation, or correspondence task needs a communication method.",
		requiredInputs: []string{
			"verified sender, recipients, audience, purpose, desired outcome, tone, and deadline",
			"source thread, attachments, commitments, relevant facts, and communication constraints",
			"send authority and review requirements",
		},
		producedOutputs: []string{
			"reviewable draft, message structure, argument map, negotiation brief, or commitment register",
			"recipient, source, claim, attachment, approval, and follow-up checks",
		},
		authorityRequirements: []string{
			"the owner or authorized delegate approves consequential external communication",
			"the verified sender controls identity, recipient list, attachments, and final send",
		},
		safetyInvariants: []string{
			"never send, publish, impersonate, or create commitments solely because a method was selected",
			"recipient and attachment verification occurs immediately before approved send",
			"factual claims remain source-grounded and uncertainty is not disguised by persuasive language",
		},
		riskCeiling: RiskHigh,
		evidenceRequirements: []string{
			"facts, quotes, commitments, and attachments link to the source thread or primary record",
			"recipient identity and send scope are verified",
			"approval records identify the approved final content and recipients",
		},
		evaluationMethod: "communication accuracy and send-safety review",
		evaluationCriteria: []string{
			"the message fits audience, purpose, tone, and requested outcome",
			"claims and commitments are supported",
			"the draft is clearly distinguished from an approved or sent communication",
		},
		methodNames: []string{
			"BLUF", "Pyramid Principle", "SCQA", "7Cs of Communication",
			"Audience-Purpose-Message", "AIDA", "PAS", "FAB",
			"Situation-Behaviour-Impact", "DESC", "Nonviolent Communication",
			"Active listening", "Reflective listening", "Motivational interviewing",
			"Steelmanning", "Argument mapping", "Negotiation preparation", "BATNA",
			"ZOPA", "Interest-based negotiation", "Difficult-conversation framework",
			"Formal-government correspondence", "Email triage",
			"Thread and commitment extraction", "Approval-before-send",
			"Recipient verification",
		},
	},
	{
		packID: PackRelationshipsCare, group: "relationships_care", domain: "Relationships and care", section: "47",
		purpose: "support consent-aware care coordination, healthy communication, boundaries, safeguarding, and shared responsibilities",
		trigger: "A relationship, conflict, caregiving, household responsibility, boundary, reminder, or safeguarding task needs a structured method.",
		requiredInputs: []string{
			"the people, relationship or care context, consent, stated needs, and desired outcome",
			"relevant commitments, boundaries, responsibilities, schedules, capacity, and support records",
			"professional or safeguarding guidance where applicable",
		},
		producedOutputs: []string{
			"consent-aware conversation plan, care plan, responsibility map, reminder, or de-escalation option",
			"explicit boundaries, responsible people, review point, uncertainty, and safeguarding route",
		},
		authorityRequirements: []string{
			"each capable person retains autonomy over their own relationships, care, data, and communication",
			"authorized caregivers and qualified professionals control delegated or clinical care decisions",
		},
		safetyInvariants: []string{
			"never manipulate, coerce, diagnose a relationship, or automate intimate communication as the owner",
			"heuristics such as Love Languages are conversational aids, not diagnoses or facts",
			"credible abuse, neglect, incapacity, or safeguarding risk overrides ordinary optimization and escalates to a human",
		},
		riskCeiling: RiskHigh,
		evidenceRequirements: []string{
			"preferences, boundaries, and commitments come from the person concerned or an authorized record",
			"care and safeguarding guidance cites qualified sources",
			"uncertain interpretation is labelled and presented for review",
		},
		evaluationMethod: "consent, care, and safeguarding review",
		evaluationCriteria: []string{
			"the output respects autonomy, consent, boundaries, and capacity",
			"responsibilities and follow-up are explicit without scorekeeping",
			"safeguarding risks have a clear human escalation path",
		},
		methodNames: []string{
			"Nonviolent Communication", "Gottman principles",
			"Active-constructive responding", "Attachment-informed communication",
			"Family-systems thinking", "Transactional Analysis",
			"Drama Triangle and Empowerment Dynamic", "Circle of control",
			"Boundary-setting", "Love Languages as a conversational heuristic",
			"Conflict de-escalation", "Restorative practices",
			"Interest-based negotiation", "Care plans", "Shared calendars",
			"Household responsibility matrices", "Reciprocity tracking without scorekeeping",
			"Important-date reminders", "Social-capacity management",
			"Safeguarding escalation",
		},
	},
	{
		packID: PackLearningGrowth, group: "learning_competence", domain: "Learning and competence", section: "48",
		purpose: "design efficient evidence-based learning, practice, assessment, and competence development",
		trigger: "A learning objective, curriculum, study, practice, assessment, skills-gap, or competence task needs a structured method.",
		requiredInputs: []string{
			"defined learning outcome, current capability, prior knowledge, constraints, and available time",
			"authoritative learning resources, practice opportunities, assessment criteria, and accessibility needs",
			"target competence level and evidence standard",
		},
		producedOutputs: []string{
			"learning objective, sequence, practice plan, assessment, competency map, or evidence portfolio",
			"measured progress, feedback, gaps, retention plan, and next practice step",
		},
		authorityRequirements: []string{
			"the learner controls personal goals, records, pacing, and consent",
			"accredited or accountable assessors control formal certification and regulated competence decisions",
		},
		safetyInvariants: []string{
			"never claim mastery, certification, or competence without matching assessment evidence",
			"learning methods adapt to accessibility, fatigue, cognitive load, and learner consent",
			"source quality is checked before information becomes a learning objective or assessment answer",
		},
		riskCeiling: RiskMedium,
		evidenceRequirements: []string{
			"learning content cites authoritative sources appropriate to the subject",
			"competence claims link to assessment or portfolio evidence",
			"progress measures preserve dates, criteria, attempts, and feedback",
		},
		evaluationMethod: "learning-outcome and retention assessment",
		evaluationCriteria: []string{
			"activities align to the stated learning objective and current level",
			"assessment measures the intended competence",
			"retention, transfer, feedback, and remaining gaps are visible",
		},
		methodNames: []string{
			"Bloom's Taxonomy", "Revised Bloom", "Feynman Technique", "Active recall",
			"Spaced repetition", "Interleaving", "Retrieval practice", "Deliberate practice",
			"Mastery learning", "Kolb learning cycle", "Experiential learning",
			"Zone of Proximal Development", "Scaffolding", "Cognitive Load Theory",
			"Dual coding", "Worked examples", "Testing effect", "Leitner system",
			"70-20-10", "Competency matrices", "Skills-gap analysis", "Learning objectives",
			"Formative and summative assessment", "Teach-back", "Portfolio-based evidence",
		},
	},
	{
		packID: PackTravelMobility, group: "travel_mobility", domain: "Travel and mobility", section: "49",
		purpose: "plan accessible, resilient, cost-aware, and energy-aware door-to-door mobility without making bookings",
		trigger: "A trip, route, transfer, fare, accessibility, disruption, packing, fatigue, or mobility task needs structured planning.",
		requiredInputs: []string{
			"travellers, origin, destination, dates, time constraints, mobility and accessibility needs",
			"live schedules, fares, weather, disruption, booking, document, luggage, and energy constraints",
			"budget and booking authority",
		},
		producedOutputs: []string{
			"source-backed itinerary, route comparison, buffer, contingency, fare option, or packing artifact",
			"accessibility, transfer, weather, fatigue, cost, and disruption risks with fallback owners",
		},
		authorityRequirements: []string{
			"the traveller or authorized coordinator approves itinerary, personal data use, booking, and spend",
			"transport operators and official authorities remain authoritative for live operation, entry, and safety rules",
		},
		safetyInvariants: []string{
			"selection never books, pays, changes, or cancels travel",
			"live schedules, weather, fares, entry rules, and disruptions are treated as time-sensitive",
			"accessibility, health, transfer, and fatigue constraints cannot be optimized away",
		},
		riskCeiling: RiskHigh,
		evidenceRequirements: []string{
			"live transport and disruption facts cite operator or official sources with retrieval time",
			"fare and booking claims include conditions, currency, scope, and expiry",
			"accessibility assumptions are confirmed with the traveller and provider",
		},
		evaluationMethod: "door-to-door feasibility and resilience check",
		evaluationCriteria: []string{
			"the itinerary is temporally feasible from origin to final destination",
			"buffers, accessibility, fatigue, documents, and fallback routes are adequate",
			"no booking or live condition is represented as confirmed without authoritative evidence",
		},
		methodNames: []string{
			"Door-to-door journey planning", "Time-dependent routing",
			"Multi-objective route optimisation", "Travelling-salesperson variants",
			"Accessibility constraints", "Transfer-risk scoring", "Buffer planning",
			"Weather-aware routing", "Fare optimisation", "Energy and fatigue planning",
			"Last-mile planning", "Contingency routes", "Geofenced reminders",
			"Packing checklists", "Live disruption management",
		},
	},
	{
		packID: PackEmergencyContinuity, group: "emergency_continuity", domain: "Emergency and continuity", section: "50",
		purpose: "prepare and coordinate human-led emergency, continuity, recovery, and after-action arrangements",
		trigger: "An emergency, threat, outage, evacuation, incapacity, recovery, fallback, or continuity task needs a structured method.",
		requiredInputs: []string{
			"current incident or planning scenario, location, affected people, animals, services, and immediate danger",
			"verified emergency contacts, professional instructions, authority, safe locations, critical records, and dependencies",
			"local emergency-service and official continuity guidance",
		},
		producedOutputs: []string{
			"human-readable emergency, evacuation, shelter, communication, recovery, or continuity artifact",
			"severity, command owner, contact path, safe action, fallback, review time, and after-action record",
		},
		authorityRequirements: []string{
			"people on scene and official emergency services control immediate life-safety action",
			"the owner or legally authorized delegate controls personal data, assets, accounts, and incapacity decisions",
		},
		safetyInvariants: []string{
			"credible immediate danger stops normal automation and directs the user to local emergency services or qualified humans",
			"never delay emergency contact to gather perfect data or complete an AI workflow",
			"never impersonate emergency services, issue false assurances, or autonomously exercise incapacity authority",
			"critical emergency data remains available offline and protected from unauthorized disclosure",
		},
		riskCeiling: RiskCritical,
		evidenceRequirements: []string{
			"emergency contacts, medical information, authority, and locations are owner-confirmed and regularly reviewed",
			"official procedures cite current local authorities or service providers",
			"incident records preserve timestamps, decisions, actors, and outcomes",
		},
		evaluationMethod: "tabletop exercise and after-action review",
		evaluationCriteria: []string{
			"people can locate and follow the plan under stress",
			"contact, authority, evacuation, shelter, recovery, and fallback paths work",
			"exercise or incident findings produce assigned corrective actions",
		},
		methodNames: []string{
			"Personal emergency plan", "Incident Command System", "Triage",
			"Severity classification", "Contact trees", "Evacuation plans",
			"Shelter-in-place plans", "Medical-information card", "Emergency funds",
			"Go-bags", "Pet emergency plans", "Utility outage plans",
			"Communication fallback", "Data backup and recovery", "Account-recovery kits",
			"Decision authority during incapacity", "After-action review",
		},
	},
}

func playbookForPack(packID PackID) DomainPlaybook {
	methods := make([]PlaybookMethod, 0)
	for _, group := range wholeLifeMethodGroups {
		if group.packID != packID {
			continue
		}
		for _, name := range group.methodNames {
			methods = append(methods, buildPlaybookMethod(group, name))
		}
	}
	playbook := DomainPlaybook{Version: PlaybookVersion, Methods: methods}
	playbook.Digest = mustPlaybookDigest(playbook)
	return playbook
}

func buildPlaybookMethod(group methodGroupDefinition, name string) PlaybookMethod {
	authority := append(cloneStrings(group.authorityRequirements),
		"method selection is advisory and grants no execution authority")
	safety := append(cloneStrings(group.safetyInvariants),
		"all execution still passes HAI constitution, mandate, risk, approval, and final-effect authorization")
	inputs := append(cloneStrings(group.requiredInputs),
		"defined objective and scope for applying "+name)
	outputs := append(cloneStrings(group.producedOutputs),
		"traceable "+name+" artifact with assumptions and unresolved questions")
	criteria := append(cloneStrings(group.evaluationCriteria),
		"the "+name+" artifact is reproducible from its cited inputs")
	return PlaybookMethod{
		ID:      group.group + "." + methodIdentifier(name),
		Version: PlaybookVersion,
		Name:    name,
		Group:   group.group,
		Domain:  group.domain,
		Purpose: "Apply " + name + " to " + group.purpose + ".",
		TriggerConditions: []string{
			group.trigger,
			"The task explicitly requests or materially benefits from " + name + ".",
			"Required inputs, evidence, and human authority can be established before consequential use.",
		},
		RequiredInputs:        inputs,
		ProducedOutputs:       outputs,
		AuthorityRequirements: authority,
		SafetyInvariants:      safety,
		RiskCeiling:           group.riskCeiling,
		EvidenceRequirements:  cloneStrings(group.evidenceRequirements),
		Evaluation: MethodEvaluation{
			Method:             group.evaluationMethod,
			Criteria:           criteria,
			FailureDisposition: "mark needs_review; do not execute, store as verified fact, or claim completion",
		},
		Provenance: MethodProvenance{
			SourceType: "owner_provided_design_specification",
			Title:      wholeLifeSpecificationTitle,
			Section:    group.section,
			Reference:  wholeLifeSpecificationRef + "/section-" + group.section,
		},
		LifecycleStatus: MethodLifecycleActive,
	}
}

func methodIdentifier(name string) string {
	var builder strings.Builder
	underscore := false
	for _, value := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			builder.WriteRune(value)
			underscore = false
			continue
		}
		if builder.Len() > 0 && !underscore {
			builder.WriteByte('_')
			underscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func mustPlaybookDigest(playbook DomainPlaybook) string {
	digest, err := playbookDigest(playbook)
	if err != nil {
		panic(err)
	}
	return digest
}

func playbookDigest(playbook DomainPlaybook) (string, error) {
	payload := struct {
		Version string           `json:"version"`
		Methods []PlaybookMethod `json:"methods"`
	}{
		Version: playbook.Version,
		Methods: playbook.Methods,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode domain playbook: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
