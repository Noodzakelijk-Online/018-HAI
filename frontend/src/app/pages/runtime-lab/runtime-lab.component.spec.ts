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

  it('does not run an unconfigured MCP server', () => {
    const { component, mcpService } = make();
    component.runMCPPreflight({ id: 'github', configured: false });

    expect(mcpService.run).not.toHaveBeenCalled();
  });
});
