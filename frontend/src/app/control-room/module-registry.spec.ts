import { HAI_MODULES, HAI_MODULE_GROUPS, moduleForUrl } from './module-registry';

describe('HAI module registry', () => {
  it('maps every registered route to a unique module', () => {
    const routes = HAI_MODULES.map((module) => module.route);

    expect(new Set(routes).size).toBe(routes.length);
    expect(HAI_MODULES.every((module) => moduleForUrl(module.route)?.id === module.id)).toBeTrue();
  });

  it('keeps navigation groups limited to operational intent', () => {
    expect(HAI_MODULE_GROUPS.map((group) => group.id)).toEqual(['work', 'intelligence', 'system']);
    expect(HAI_MODULES.every((module) => HAI_MODULE_GROUPS.some((group) => group.id === module.group))).toBeTrue();
  });

  it('registers the guarded Framework Registry route with its shell contract', () => {
    const frameworkRegistry = HAI_MODULES.find(
      (module) => module.route === '/framework-registry'
    );

    expect(frameworkRegistry).toEqual(
      jasmine.objectContaining({
        id: 'framework-registry',
        group: 'system',
        title: 'Framework registry',
        icon: 'apartment',
        description: 'Governed framework selection, owner preferences, and Constitution.',
        primaryAction: {
          label: 'Select frameworks',
          capability: 'framework-registry:select',
        },
        advancedSectionIds: [
          'selection-context',
          'selection-history',
          'constitution-history',
          'constitution-governance',
        ],
      })
    );
    expect(moduleForUrl('/framework-registry?mode=advanced#constitution').id)
      .toBe('framework-registry');
  });

  it('registers Life Ops with owner-context progressive sections', () => {
    const lifeOps = HAI_MODULES.find((module) => module.route === '/life-ops');

    expect(lifeOps).toEqual(jasmine.objectContaining({
      id: 'life-ops',
      group: 'intelligence',
      title: 'Life Ops',
      primaryAction: {
        label: 'Record current context',
        capability: 'life-ops:write',
      },
      advancedSectionIds: [
        'need-observations',
        'capacity-record',
        'entity-domains',
        'goal-hierarchy',
        'priority-assessment',
        'provenance',
      ],
    }));
    expect(moduleForUrl('/life-ops?mode=advanced#priority-assessment').id)
      .toBe('life-ops');
  });
});
