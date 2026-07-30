export type HaiModuleGroup = 'work' | 'intelligence' | 'system'

export interface HaiModuleDefinition {
  id: string
  route: string
  group: HaiModuleGroup
  title: string
  description: string
  icon: string
}

export const HAI_MODULES: HaiModuleDefinition[] = [
  { id: 'control-center', route: '/control-center', group: 'work', title: 'Command Center', description: 'Your next useful action.', icon: 'appstore' },
  { id: 'automations', route: '/home', group: 'work', title: 'Automations', description: 'Manage approved automation work.', icon: 'setting' },
  { id: 'background-operations', route: '/background-operations', group: 'work', title: 'Background operations', description: 'Safe work running while you are away.', icon: 'thunderbolt' },
  { id: 'pursuits', route: '/pursuits', group: 'work', title: 'Pursuits', description: 'Long-running goals and their next moves.', icon: 'flag' },
  { id: 'workflow-engine', route: '/workflow-engine', group: 'work', title: 'Workflows', description: 'Review and move controlled workflows forward.', icon: 'unordered-list' },
  { id: 'quick-capture', route: '/quick-capture', group: 'work', title: 'Quick capture', description: 'Turn a thought into controlled work.', icon: 'plus-square' },
  { id: 'exceptions', route: '/exceptions', group: 'work', title: 'Exceptions', description: 'Resolve work that needs intervention.', icon: 'warning' },
  { id: 'task-blueprint', route: '/task-blueprint', group: 'intelligence', title: 'Task planning', description: 'Talk to HAI and inspect its plan.', icon: 'partition' },
  { id: 'connected-sources', route: '/connected-sources', group: 'intelligence', title: 'Sources', description: 'Connected accounts, files, and sync health.', icon: 'cluster' },
  { id: 'account-bridges', route: '/account-bridges', group: 'intelligence', title: 'Account bridges', description: 'Connection and permission health.', icon: 'link' },
  { id: 'memory', route: '/memory', group: 'intelligence', title: 'Memory', description: 'Review useful, source-linked context.', icon: 'database' },
  { id: 'grounded-answers', route: '/grounded-answers', group: 'intelligence', title: 'Verified answers', description: 'Evidence, claims, and verification.', icon: 'safety-certificate' },
  { id: 'ambient-brain', route: '/ambient-brain', group: 'intelligence', title: 'Brain settings', description: 'Priorities, safeguards, and proactive work.', icon: 'compass' },
  { id: 'brain-catalog', route: '/brain-catalog', group: 'intelligence', title: 'Brain catalog', description: 'Reviewed external capabilities and activation gates.', icon: 'book' },
  { id: 'hai-os', route: '/hai-os', group: 'intelligence', title: 'HAI OS', description: 'Operating-system architecture and readiness.', icon: 'deployment-unit' },
  { id: 'llm-policy', route: '/llm-policy', group: 'system', title: 'Models', description: 'Local-first routing, providers, and budget.', icon: 'deployment-unit' },
  { id: 'model-intelligence', route: '/model-intelligence', group: 'system', title: 'Model intelligence', description: 'Provider and capability health.', icon: 'experiment' },
  { id: 'runtime-control', route: '/runtime-control', group: 'system', title: 'Runtime control', description: 'Controlled execution and safety gates.', icon: 'poweroff' },
  { id: 'runtime-lab', route: '/runtime-lab', group: 'system', title: 'Runtime lab', description: 'Runtime adapters and capability checks.', icon: 'api' },
  { id: 'system-status', route: '/system-status', group: 'system', title: 'System status', description: 'Services, dependencies, and health.', icon: 'heart' },
  { id: 'command-dashboard', route: '/command-dashboard', group: 'system', title: 'Legacy dashboard', description: 'Detailed operating records.', icon: 'dashboard' },
]

export const HAI_MODULE_GROUPS: Array<{ id: HaiModuleGroup; label: string }> = [
  { id: 'work', label: 'Work' },
  { id: 'intelligence', label: 'Intelligence' },
  { id: 'system', label: 'System' },
]

export function moduleForUrl(url: string): HaiModuleDefinition {
  const path = url.split('?')[0].split('#')[0]
  return HAI_MODULES.find((module) => path === module.route || path.startsWith(`${module.route}/`)) || HAI_MODULES[0]
}
