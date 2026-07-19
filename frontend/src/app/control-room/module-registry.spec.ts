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
});
