import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { Subject } from 'rxjs';
import { IConnectedSource, ISourcePursuitRoutingOutcome } from '../../models/connected-source.model.interface';
import { ConnectedSourcesComponent } from './connected-sources.component';

describe('ConnectedSourcesComponent pursuit handoff', () => {
  function createComponent(): { component: ConnectedSourcesComponent; router: jasmine.SpyObj<Router>; sourceService: jasmine.SpyObj<any> } {
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
	const sourceService = jasmine.createSpyObj('ConnectedSourceService', ['createSource']);
    return {
      component: new ConnectedSourcesComponent(
        new FormBuilder(),
		sourceService,
        {} as NzNotificationService,
		router,
        { mode: () => 'light' } as any,
      ),
      router,
	  sourceService,
    };
  }

  it('opens the candidate pursuit returned by a source sync', () => {
    const { component, router } = createComponent();
    const outcome: ISourcePursuitRoutingOutcome = {
      extractionId: 'extraction-1',
      pursuitId: '68882979-1333-4b14-bd46-16fd5806c9c5',
      status: 'candidate_pending',
      message: 'Imported source candidate awaits explicit acceptance.',
    };

    component.lastSyncResult = {
      job: {} as any,
      extractions: [],
      pursuitOutcomes: [outcome],
      message: 'sync complete',
    };
    component.openPursuitOutcome(outcome);

    expect(component.pursuitRoutingOutcomes()).toEqual([outcome]);
    expect(component.pursuitRoutingLabel(outcome)).toBe('Decision needed');
    expect(router.navigate).toHaveBeenCalledWith(['/pursuits'], { queryParams: { selected: outcome.pursuitId } });
  });

  it('opens the pursuits inbox when a routing repair has no pursuit ID', () => {
    const { component, router } = createComponent();
    component.openPursuitOutcome({ status: 'routing_deferred', message: 'Router repair is required.' });

    expect(router.navigate).toHaveBeenCalledWith(['/pursuits'], { queryParams: undefined });
  });

  it('opens the private graph inspector for projected source records', () => {
    const { component, router } = createComponent();
    component.lastSyncResult = {
      job: {} as any,
      extractions: [],
      message: 'sync complete',
      lifeGraphProjections: [{
        extractionId: 'extraction-1', documentId: 'life-document-1', linkedEntityIds: ['source-1'],
        relationIds: ['relation-1'], alreadyExisted: false, advisoryOnly: true,
        canExecute: false, grantsAuthority: false,
      }],
    };

    component.openLifeGraph();

    expect(component.lifeGraphProjections().length).toBe(1);
    expect(router.navigate).toHaveBeenCalledWith(['/governance-control']);
  });

  it('recognizes all read-only Google source types', () => {
    const { component } = createComponent();
    expect(component.isGoogleSource({ connectorKey: 'gmail' } as IConnectedSource)).toBeTrue();
    expect(component.isGoogleSource({ connectorKey: 'google-drive' } as IConnectedSource)).toBeTrue();
    expect(component.isGoogleSource({ connectorKey: 'google-contacts' } as IConnectedSource)).toBeTrue();
    expect(component.isGoogleSource({ connectorKey: 'google-calendar' } as IConnectedSource)).toBeTrue();
    expect(component.isGoogleSource({ connectorKey: 'github' } as IConnectedSource)).toBeFalse();
  });

  it('reports Google readiness independently from other live connectors', () => {
    const { component } = createComponent();
    component.connectors = [
      { connectorKey: 'github', enabled: true, adapterStatus: 'operational' } as any,
      { connectorKey: 'gmail', enabled: true, adapterStatus: 'not_implemented' } as any,
      { connectorKey: 'google-drive', enabled: true, adapterStatus: 'not_implemented' } as any,
      { connectorKey: 'google-contacts', enabled: true, adapterStatus: 'not_implemented' } as any,
      { connectorKey: 'google-calendar', enabled: true, adapterStatus: 'not_implemented' } as any,
    ];

    expect(component.googleConnectorMetric()).toBe('setup needed');

    component.connectors[1].adapterStatus = 'operational';
    expect(component.googleConnectorMetric()).toBe('1/4 ready');

    component.connectors[2].adapterStatus = 'operational';
    expect(component.googleConnectorMetric()).toBe('2/4 ready');

    component.connectors[3].adapterStatus = 'operational';
    expect(component.googleConnectorMetric()).toBe('3/4 ready');

    component.connectors[4].adapterStatus = 'operational';
    expect(component.googleConnectorMetric()).toBe('4 ready');
  });

  it('does not offer connector creation before required setup is complete', () => {
		const { component } = createComponent();
		expect(component.connectorCanConnect({ enabled: true, adapterStatus: 'configuration_required' } as any)).toBeFalse();
		expect(component.connectorCanConnect({ enabled: true, adapterStatus: 'not_implemented' } as any)).toBeFalse();
		expect(component.connectorCanConnect({ enabled: true, adapterStatus: 'operational' } as any)).toBeTrue();
		expect(component.adapterStatusLabel('configuration_required')).toBe('setup required');
	});

  it('keeps Google connection controls disabled until that connector is configured', () => {
    const { component } = createComponent();
    component.connectors = [
      { connectorKey: 'gmail', enabled: true, adapterStatus: 'configuration_required' } as any,
      { connectorKey: 'google-drive', enabled: true, adapterStatus: 'operational' } as any,
    ];

    expect(component.googleConnectorCanConnect('gmail')).toBeFalse();
    expect(component.googleConnectorCanConnect('google-drive')).toBeTrue();
  });

  it('uses remote read-only defaults for a Trello board source', () => {
    const { component } = createComponent();

    component.sourceForm.patchValue({ connectorKey: 'trello' });
    component.connectorChanged('trello');

    expect(component.sourceForm.value).toEqual(jasmine.objectContaining({
      name: 'Trello board (read-only)',
      syncFrequency: '1h',
      syncTarget: '',
      localOnly: false,
    }));
    expect(component.syncTargetPlaceholder()).toContain('Trello board URL or ID');
  });

	it('marks generic source creation as in progress until the request completes', () => {
		const { component, sourceService } = createComponent();
		component.connectors = [{ connectorKey: 'local-folder', enabled: true, adapterStatus: 'local_only', category: 'local_folder' } as any];
		component.sourceForm.patchValue({ connectorKey: 'local-folder' });
		const pending = new Subject<any>();
		sourceService.createSource.and.returnValue(pending.asObservable());
		component.connectSource();
		expect(sourceService.createSource).toHaveBeenCalled();
		expect(component.connecting).toBeTrue();
		pending.complete();
		expect(component.connecting).toBeFalse();
	});
});
