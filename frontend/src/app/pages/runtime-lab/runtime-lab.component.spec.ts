import { of } from 'rxjs';
import { RuntimeLabComponent } from './runtime-lab.component';

describe('RuntimeLabComponent MCP readiness', () => {
  function make() {
    const runtimeService = jasmine.createSpyObj('RuntimeLabService', ['overview', 'featureParity', 'capabilities', 'probe', 'selfTest']);
    const mcpService = jasmine.createSpyObj('MCPPreflightService', ['overview', 'run']);
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'warning', 'error', 'info']);
    const router = jasmine.createSpyObj('Router', ['navigate']);
    runtimeService.overview.and.returnValue(of({ runtimes: [] }));
    runtimeService.featureParity.and.returnValue(of({
      requiredCoverageAreas: ['agent_runtimes'],
      inventories: [{
        runtimeId: 'openclaw',
        project: 'OpenClaw',
        repositoryUrl: 'https://github.com/openclaw/openclaw',
        defaultBranch: 'main',
        reviewedRevision: 'abc',
        reviewedAt: '2026-08-08T00:00:00Z',
        license: 'MIT',
        licensePolicy: 'Remote protocol only.',
        readinessCeiling: 'declared',
        canonicalAuthority: 'HAI',
        features: [],
      }],
      dispositionCounts: {},
      implementationCounts: {},
      generatedAt: '2026-08-08T00:00:00Z',
    }));
    runtimeService.capabilities.and.returnValue(of({
      cards: [{
        id: 'openclaw.gateway.discovery',
        runtimeId: 'openclaw',
        name: 'Inspect OpenClaw Gateway readiness',
        purpose: 'Read metadata only.',
        inputSchema: { type: 'object' },
        outputSchema: { type: 'object' },
        authenticationState: 'not_configured',
        availability: 'not_configured',
        runtimeLocation: 'operator_managed_local_service',
        requiredAuthority: ['authenticated_owner', 'runtime.read'],
        riskLevel: 'low',
        expectedCostEurMax: 0,
        costPolicy: 'No paid calls.',
        contextCost: 'none',
        timeoutSeconds: 5,
        retryBehaviour: 'manual',
        reversibility: 'read_only',
        approvalRequirements: ['owner managed'],
        verificationMethod: 'schema check',
        evidenceReturned: ['identity'],
        readinessLevel: 'declared',
        readinessReason: 'Contract only.',
        canInvoke: false,
        canExecuteExternalEffect: false,
        sourceFeatureIds: ['openclaw-gateway'],
      }],
      counts: { declared: 1 },
      authority: 'contract_only',
      safetyNote: 'No permission is granted.',
    }));
    mcpService.overview.and.returnValue(of({ enabled: true, scope: 'Read-only MCP preflight.', servers: [{ id: 'github', catalogName: 'GitHub MCP Server', configured: true }] }));
    return { component: new RuntimeLabComponent(runtimeService, mcpService, notification, router), mcpService, notification };
  }

  it('loads MCP readiness alongside runtime summaries', () => {
    const { component, mcpService } = make();
    component.refresh();

    expect(mcpService.overview).toHaveBeenCalled();
    expect(component.mcpOverview?.servers[0].catalogName).toBe('GitHub MCP Server');
  });

  it('loads the source-reviewed runtime parity inventory without changing readiness', () => {
    const { component } = make();
    component.refresh();

    expect(component.parityOverview?.inventories[0].runtimeId).toBe('openclaw');
    expect(component.parityOverview?.inventories[0].readinessCeiling).toBe('declared');
    expect(component.parityLoading).toBeFalse();
  });

  it('loads non-executable HAI capability cards separately from runtime claims', () => {
    const { component } = make();
    component.refresh();

    const cards = component.cardsForRuntime('openclaw');
    expect(cards.length).toBe(1);
    expect(cards[0].readinessLevel).toBe('declared');
    expect(cards[0].canInvoke).toBeFalse();
    expect(cards[0].canExecuteExternalEffect).toBeFalse();
  });

  it('labels and counts runtime dispositions for progressive inspection', () => {
    const { component } = make();
    const inventory: any = {
      features: [
        { implementationStatus: 'implemented', disposition: 'already_present' },
        { implementationStatus: 'not_implemented', disposition: 'deferred' },
        { implementationStatus: 'not_implemented', disposition: 'blocked_external' },
      ],
    };

    expect(component.implementationCount(inventory, 'implemented')).toBe(1);
    expect(component.backlogCount(inventory)).toBe(2);
    expect(component.dispositionColor('constrained_unsafe')).toBe('red');
    expect(component.dispositionLabel('adapted_for_hai')).toBe('adapted for hai');
  });

  it('runs only the selected configured MCP preflight', () => {
    const { component, mcpService, notification } = make();
    mcpService.run.and.returnValue(of({ status: 'ready', toolCount: 3, detail: 'No tool was called.' }));
    component.runMCPPreflight({ id: 'github', catalogName: 'GitHub MCP Server', configured: true });

    expect(mcpService.run).toHaveBeenCalledWith('github');
    expect(notification.success).toHaveBeenCalledWith('MCP server ready', 'GitHub MCP Server: 3 declared tool(s) inspected. No tool was called.');
  });

  it('does not run an unconfigured MCP server', () => {
    const { component, mcpService } = make();
    component.runMCPPreflight({ id: 'github', configured: false });

    expect(mcpService.run).not.toHaveBeenCalled();
  });
});
