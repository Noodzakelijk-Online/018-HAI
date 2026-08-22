import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { of, Subject } from 'rxjs';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { IConnectedSource, ISourcePursuitRoutingOutcome } from '../../models/connected-source.model.interface';
import { ConnectedSourcesComponent } from './connected-sources.component';

describe('ConnectedSourcesComponent pursuit handoff', () => {
  function createComponent(): { component: ConnectedSourcesComponent; router: jasmine.SpyObj<Router> } {
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    return {
      component: new ConnectedSourcesComponent(
        new FormBuilder(),
        {} as any,
        {} as NzNotificationService,
        router,
        { mode: () => 'light' } as any,
      ),
      router,
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

  it('labels configuration-required connectors as setup required and prevents source creation', () => {
    const { component } = createComponent();
    const connector = {
      connectorKey: 'trello',
      enabled: true,
      adapterStatus: 'configuration_required',
    } as any;

    expect(component.adapterStatusLabel(connector.adapterStatus)).toBe('setup required');
    expect(component.connectorCanCreateSource(connector)).toBeFalse();
  });

  it('keeps enabled legacy connectors usable when the backend omits adapter status', () => {
    const { component } = createComponent();

    expect(component.connectorCanCreateSource({ connectorKey: 'local-folder', enabled: true } as any)).toBeTrue();
  });

  it('prevents duplicate generic source creation while the request is running', () => {
    const { component } = createComponent();
    const pending = new Subject<IConnectedSource>();
    const sourceService = jasmine.createSpyObj('ConnectedSourceService', ['createSource']);
    sourceService.createSource.and.returnValue(pending.asObservable());
    (component as any).sourceService = sourceService;
    component.connectors = [{
      connectorKey: 'local-folder', name: 'Selected local folder', category: 'local_folder',
      enabled: true, adapterStatus: 'local_only',
    } as any];

    component.connectSource();
    component.connectSource();

    expect(component.connecting).toBeTrue();
    expect(sourceService.createSource).toHaveBeenCalledTimes(1);

    pending.complete();
    expect(component.connecting).toBeFalse();
  });

  it('initializes the live Trello connector as a non-local, scheduled board source', () => {
    const { component } = createComponent();
    component.sourceForm.patchValue({
      connectorKey: 'trello',
      localOnly: true,
      syncTarget: 'documents',
      excludePatterns: 'trash,temp',
    });

    component.connectorChanged('trello');

    expect(component.sourceForm.value).toEqual(jasmine.objectContaining({
      connectorKey: 'trello',
      name: 'Trello board (read-only API)',
      localOnly: false,
      syncFrequency: '15m',
      syncTarget: '',
      excludePatterns: '',
    }));
    expect(component.syncTargetPlaceholder()).toContain('Trello board ID');
  });

  it('defers record-heavy source history until the operator opens it', () => {
    const { component } = createComponent();
    const sourceService = (component as any).sourceService = jasmine.createSpyObj('ConnectedSourceService', [
      'connectors', 'sources', 'pageExtractions', 'pageAuditLogs', 'pageSyncJobs', 'connectionHealthSummary',
    ]);
    sourceService.connectors.and.returnValue(of([]));
    sourceService.sources.and.returnValue(of([]));
    sourceService.pageExtractions.and.returnValue(of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }));
    sourceService.pageAuditLogs.and.returnValue(of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }));
    sourceService.pageSyncJobs.and.returnValue(of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }));
    sourceService.connectionHealthSummary.and.returnValue(of([]));

    component.refresh();

    expect(sourceService.pageExtractions).not.toHaveBeenCalled();
    expect(sourceService.pageAuditLogs).not.toHaveBeenCalled();
    expect(sourceService.pageSyncJobs).not.toHaveBeenCalled();
    expect(component.recordHistoryMetric()).toBe('open');

    component.loadRecordHistory();

    expect(sourceService.pageExtractions).toHaveBeenCalledWith('018-HAI', false, 100, 0);
    expect(sourceService.pageAuditLogs).toHaveBeenCalledWith(100, 0);
    expect(sourceService.pageSyncJobs).toHaveBeenCalledWith(100, 0);
    expect(component.recordHistoryLoaded).toBeTrue();
  });

  it('loads the next bounded history page only when older records are requested', () => {
    const { component } = createComponent();
    const sourceService = (component as any).sourceService = jasmine.createSpyObj('ConnectedSourceService', [
      'pageExtractions', 'pageAuditLogs', 'pageSyncJobs',
    ]);
    sourceService.pageExtractions.and.returnValues(
      of({ items: [{ id: 'first' }], total: 2, limit: 100, offset: 0, hasMore: true }),
      of({ items: [{ id: 'second' }], total: 2, limit: 100, offset: 1, hasMore: false }),
    );
    sourceService.pageAuditLogs.and.returnValues(
      of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }),
      of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }),
    );
    sourceService.pageSyncJobs.and.returnValues(
      of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }),
      of({ items: [], total: 0, limit: 100, offset: 0, hasMore: false }),
    );

    component.loadRecordHistory();
    component.loadOlderRecordHistory();

    expect(sourceService.pageExtractions).toHaveBeenCalledWith('018-HAI', false, 100, 1);
    expect(component.extractions.map((item) => item.id)).toEqual(['first', 'second']);
    expect(component.recordHistoryMetric()).toBe('2');
    expect(component.recordHistoryHasMore()).toBeFalse();
  });
});
