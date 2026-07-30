package lifeops

var canonicalLifeDomains = []LifeDomain{
	{ID: DomainLegalGovernment, Name: "Legal and government", Description: "Legal cases, government obligations, insurance, rights, and formal disputes.", NeedClass: "rights_and_security", Sensitive: true},
	{ID: DomainEmergencyContinuity, Name: "Emergency and continuity", Description: "Immediate safety, incapacity, disaster recovery, and critical continuity.", NeedClass: "safety_and_stability", Sensitive: true},
	{ID: DomainHealthWellbeing, Name: "Health and wellbeing", Description: "Physical and mental health, treatment, sleep, stress, and care plans.", NeedClass: "physiological_and_wellbeing", Sensitive: true},
	{ID: DomainFinancial, Name: "Financial", Description: "Income, budgets, payments, debt, tax, liquidity, and financial commitments.", NeedClass: "safety_and_stability", Sensitive: true},
	{ID: DomainWorkVenture, Name: "Work and ventures", Description: "Employment, clients, products, services, businesses, and professional delivery.", NeedClass: "esteem_and_material_progress"},
	{ID: DomainHomeAssets, Name: "Home and assets", Description: "Housing, garden, maintenance, repairs, vehicles, and major assets.", NeedClass: "safety_and_stability"},
	{ID: DomainRelationshipsCare, Name: "Relationships and care", Description: "Partners, friends, caregiving, interpersonal commitments, and conflict.", NeedClass: "belonging_and_care", Sensitive: true},
	{ID: DomainLearningGrowth, Name: "Learning and growth", Description: "Education, training, skills, research, and competence development.", NeedClass: "competence_and_growth"},
	{ID: DomainTravelMobility, Name: "Travel and mobility", Description: "Transport, trips, routes, accommodation, visas, and movement.", NeedClass: "mobility_and_autonomy"},
	{ID: DomainPersonalProductivity, Name: "Personal productivity", Description: "Tasks, focus, routines, attention, reviews, and personal execution.", NeedClass: "self_management"},
	{ID: DomainIdentityRoles, Name: "Identity and roles", Description: "Identity records, personal roles, profiles, and role responsibilities.", NeedClass: "identity_and_autonomy", Sensitive: true},
	{ID: DomainFamilyHousehold, Name: "Family and household", Description: "Family plans, household members, shared schedules, and responsibilities.", NeedClass: "belonging_and_care", Sensitive: true},
	{ID: DomainFoodNutrition, Name: "Food and nutrition", Description: "Meals, groceries, nutrition, dietary needs, and cooking.", NeedClass: "physiological_and_nutrition", Sensitive: true},
	{ID: DomainCommunication, Name: "Communication and correspondence", Description: "Email, letters, messages, calls, inboxes, and formal correspondence.", NeedClass: "connection_and_participation", Sensitive: true},
	{ID: DomainDigitalAccounts, Name: "Digital accounts", Description: "Online accounts, authentication, subscriptions, access, and digital identity.", NeedClass: "safety_and_stability", Sensitive: true},
	{ID: DomainPossessionsInventory, Name: "Possessions and inventory", Description: "Equipment, possessions, tools, storage, serial numbers, and inventories.", NeedClass: "material_stability"},
	{ID: DomainAnimalsDependants, Name: "Animals and dependants", Description: "Pets, animal care, and other dependant-care obligations.", NeedClass: "belonging_and_care", Sensitive: true},
	{ID: DomainCommunityCivic, Name: "Community and civic life", Description: "Neighbourhood, volunteering, politics, public consultation, and civic duties.", NeedClass: "connection_and_participation"},
	{ID: DomainLeisureRecreation, Name: "Leisure and recreation", Description: "Rest, hobbies, recreation, recovery, and enjoyable activities.", NeedClass: "rest_and_recreation"},
	{ID: DomainCreativityExpression, Name: "Creativity and expression", Description: "Art, music, writing, photography, design, and creative practice.", NeedClass: "competence_and_growth"},
	{ID: DomainMeaningValues, Name: "Meaning and values", Description: "Purpose, values, spirituality, reflection, and personal meaning.", NeedClass: "meaning_and_legacy", Sensitive: true},
	{ID: DomainEnvironmentSustainability, Name: "Environment and sustainability", Description: "Energy, recycling, carbon, biodiversity, and environmental stewardship.", NeedClass: "environment_and_stewardship"},
	{ID: DomainLegacyLongTerm, Name: "Legacy and long term", Description: "Succession, estate planning, archives, future generations, and durable legacy.", NeedClass: "meaning_and_legacy", Sensitive: true},
	{ID: DomainSafetySecurity, Name: "Safety and security", Description: "Personal safety, threats, incidents, protective measures, and security.", NeedClass: "safety_and_stability", Sensitive: true},
}

var canonicalDomainIndex = func() map[DomainID]LifeDomain {
	result := make(map[DomainID]LifeDomain, len(canonicalLifeDomains))
	for _, domain := range canonicalLifeDomains {
		result[domain.ID] = domain
	}
	return result
}()

func CanonicalLifeDomains() []LifeDomain {
	return append([]LifeDomain(nil), canonicalLifeDomains...)
}

func FindLifeDomain(id DomainID) (LifeDomain, bool) {
	domain, ok := canonicalDomainIndex[id]
	return domain, ok
}

func IsCanonicalDomain(id DomainID) bool {
	_, ok := canonicalDomainIndex[id]
	return ok
}

var goalLevelRanks = map[GoalLevel]int{
	GoalLevelValues:           1,
	GoalLevelNeeds:            2,
	GoalLevelVision:           3,
	GoalLevelStrategicOutcome: 4,
	GoalLevelPursuit:          5,
	GoalLevelProgrammeCase:    6,
	GoalLevelProject:          7,
	GoalLevelWorkflow:         8,
	GoalLevelTask:             9,
	GoalLevelAtomicAction:     10,
	GoalLevelVerification:     11,
	GoalLevelMeasuredOutcome:  12,
}

func GoalLevelRank(level GoalLevel) (int, bool) {
	rank, ok := goalLevelRanks[level]
	return rank, ok
}
