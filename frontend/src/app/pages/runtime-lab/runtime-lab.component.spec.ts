import { of } from 'rxjs';
import { RuntimeLabComponent } from './runtime-lab.component';

describe('RuntimeLabComponent MCP readiness', () => {
  function make() {
    const runtimeService = jasmine.createSpyObj('RuntimeLabService', ['overview', 'probe', 'selfTest']);
    const mcpService = jasmine.createSpyObj('MCPPreflightService', ['overview', 'run']);
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'warning', 'error', 'info']);
    const router = jasmine.createSpyObj('Router', ['navigate']);
    runtimeService.overview.and.returnValue(of({ runtimes: [] }));
    mcpService.overview.and.returnValue(of({ enabled: true, scope: 'Read-only MCP preflight.', servers: [{ id: 'github', catalogName: 'GitHub MCP Server', configured: true }] }));
    return { component: new RuntimeLabComponent(runtimeService, mcpService, notification, router), mcpService, notification };
  }

  it('loads MCP readiness alongside runtime summaries', () => {
    const { component, mcpService } = make();
    component.refresh();

    expect(mcpService.overview).toHaveBeenCalled();
    expect(component.mcpOverview?.servers[0].catalogName).toBe('GitHub MCP Server');
  });

  it('runs only the selected configured MCP preflight', () => {
    const { component, mcpService, notification } = make();
    mcpService.run.and.returnValue(of({ status: 'ready', toolCount: 3, detail: 'No tool was called.' }));
    component.runMCPPreflight({ id: 'github', catalogName: 'GitHub MCP Server', configured: true });

    expect(mcpService.run).toHaveBeenCalledWith('github');
    expect(notification.success).toHaveBeenCalledWith('MCP server ready', 'GitHub MCP Server: 3 declared tool(s) inspected. No tool was called.');
  });

  it('identifies an inventory that passed the GitHub read-only contract', () => {
    const { component, mcpService, notification } = make();
    mcpService.run.and.returnValue(of({ status: 'ready', toolCount: 2, detail: 'No tool was called.', readOnlyVerified: true }));
    component.runMCPPreflight({ id: 'github', catalogName: 'GitHub MCP Server', configured: true });

    expect(notification.success).toHaveBeenCalledWith(
      'MCP server ready',
      'GitHub MCP Server: 2 declared tool(s) inspected. No tool was called. Declared tools matched HAI\'s inspection-only context contract.',
    );
  });

  it('recognizes the reviewed GitHub and Playwright inspection profiles', () => {
    const { component } = make();
    expect(component.isReadOnlyContractServer({ id: 'github', catalogId: 'github-mcp-server', configured: true })).toBeTrue();
    expect(component.isReadOnlyContractServer({ id: 'playwright', catalogId: 'playwright-mcp', configured: true })).toBeTrue();
    expect(component.isReadOnlyContractServer({ id: 'serena', catalogId: 'serena', configured: true })).toBeFalse();
  });

  it('does not run an unconfigured MCP server', () => {
    const { component, mcpService } = make();
    component.runMCPPreflight({ id: 'github', configured: false });

    expect(mcpService.run).not.toHaveBeenCalled();
  });
});
