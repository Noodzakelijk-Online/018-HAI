import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { Subject, of, throwError } from 'rxjs';
import { IConnectedSource, ISourcePursuitRoutingOutcome } from '../../models/connected-source.model.interface';
import { ConnectedSourcesComponent } from './connected-sources.component';

describe('ConnectedSourcesComponent pursuit handoff', () => {
  function createComponent(): { component: ConnectedSourcesComponent; router: jasmine.SpyObj<Router>; sourceService: jasmine.SpyObj<any> } {
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['info', 'error', 'warning']);
    const sourceService = jasmine.createSpyObj('ConnectedSourceService', [
      'createSource', 'sync', 'transcribe', 'extractDocuments', 'runDueScheduledSyncs',
      'connectors', 'sources', 'extractions', 'auditLogs', 'syncJobs', 'connectionHealth', 'connectionHealths', 'startGoogleOAuth'
    ]);
    return {
      component: new ConnectedSourcesComponent(
        new FormBuilder(),
		sourceService,
		notification,
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

  it('uses a reliable same-tab handoff for Google authorization', () => {
    const { component, sourceService } = createComponent();
    sourceService.startGoogleOAuth.and.returnValue(of({ authorizeUrl: 'https://accounts.google.test/authorize' }));
    const navigate = spyOn<any>(component, 'navigateToGoogleAuthorization');

    component.authorizeGoogleSource({ id: 'source-id', connectorKey: 'gmail' } as IConnectedSource);

    expect(sourceService.startGoogleOAuth).toHaveBeenCalledWith('source-id');
    expect(navigate).toHaveBeenCalledWith('https://accounts.google.test/authorize');
  });

  it('records HAI normalized read permissions before Gmail OAuth starts', () => {
    const { component, sourceService } = createComponent();
    component.connectors = [{ connectorKey: 'gmail', category: 'email', enabled: true, adapterStatus: 'operational' } as any];
    sourceService.createSource.and.returnValue(of({ id: 'gmail-source', connectorKey: 'gmail' }));
    sourceService.startGoogleOAuth.and.returnValue(of({ authorizeUrl: 'https://accounts.google.test/authorize' }));
    spyOn<any>(component, 'navigateToGoogleAuthorization');
    spyOn(component, 'refresh');

    component.connectGmail();

    expect(sourceService.createSource).toHaveBeenCalledWith(jasmine.objectContaining({
      connectorKey: 'gmail',
      permissions: ['metadata:read', 'email:read'],
    }));
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

  it('does not start duplicate source work while another sync is running', () => {
    const { component, sourceService } = createComponent();
    const pending = new Subject<any>();
    sourceService.sync.and.returnValue(pending.asObservable());
    sourceService.transcribe.and.returnValue(pending.asObservable());
    sourceService.extractDocuments.and.returnValue(pending.asObservable());
    sourceService.runDueScheduledSyncs.and.returnValue(pending.asObservable());
    component.importForm.patchValue({ sourceId: 'import-source' });
    component.folderForm.patchValue({ sourceId: 'folder-source' });
    component.syncing = true;

    component.sync();
    component.syncFolder();
    component.runDueScheduledSyncs();
    component.syncSource({ id: 'github-source', connectorKey: 'github' } as IConnectedSource);
    component.syncSource({ id: 'audio-source', connectorKey: 'whisper-audio' } as IConnectedSource);
    component.syncSource({ id: 'document-source', connectorKey: 'docling-documents' } as IConnectedSource);

    expect(sourceService.sync).not.toHaveBeenCalled();
    expect(sourceService.transcribe).not.toHaveBeenCalled();
    expect(sourceService.extractDocuments).not.toHaveBeenCalled();
    expect(sourceService.runDueScheduledSyncs).not.toHaveBeenCalled();
  });

  it('keeps failed auxiliary records visible instead of presenting them as empty', () => {
    const { component, sourceService } = createComponent();
    sourceService.connectors.and.returnValue(of([]));
    sourceService.sources.and.returnValue(of([]));
    sourceService.extractions.and.returnValue(throwError(() => new Error('extractions unavailable')));
    sourceService.auditLogs.and.returnValue(throwError(() => new Error('audit unavailable')));
    sourceService.syncJobs.and.returnValue(throwError(() => new Error('jobs unavailable')));

    component.refresh();

    expect(component.loadWarnings).toEqual([
      'Extracted records',
      'Audit history',
      'Sync jobs',
    ]);
    expect(component.hasLoadWarnings()).toBeTrue();
  });

  it('marks an unavailable source health batch instead of treating it as absent health', () => {
    const { component, sourceService } = createComponent();
    const source = { id: 'gmail-source', connectorKey: 'gmail', name: 'Personal Gmail' } as IConnectedSource;
    sourceService.connectionHealths.and.returnValue(throwError(() => new Error('health unavailable')));

    (component as any).loadConnectionHealth([source]);

    expect(sourceService.connectionHealths).toHaveBeenCalledTimes(1);
    expect(component.sourceHealthUnavailable(source)).toBeTrue();
    expect(component.sourceHealth(source)).toBeUndefined();
  });

  it('builds constant-time source record and latest-job lookups', () => {
    const { component } = createComponent();
    const source = { id: 'gmail-source', connectorKey: 'gmail', name: 'Personal Gmail' } as IConnectedSource;
    component.extractions = [
      { id: 'record-1', sourceId: source.id } as any,
      { id: 'record-2', sourceId: source.id } as any,
    ];
    component.syncJobs = [
      { id: 'newest-job', sourceId: source.id, status: 'completed' } as any,
      { id: 'older-job', sourceId: source.id, status: 'failed' } as any,
    ];

    (component as any).rebuildSourceIndexes();

    expect(component.sourceExtractionCount(source)).toBe(2);
    expect(component.latestJobFor(source)?.id).toBe('newest-job');
  });

  it('preserves extracted records when an action-triggered reload fails', () => {
    const { component, sourceService } = createComponent();
    component.extractions = [{ id: 'existing-record' } as any];
    sourceService.extractions.and.returnValue(throwError(() => new Error('records unavailable')));

    (component as any).loadExtractions();

    expect(component.extractions.map((item) => item.id)).toEqual(['existing-record']);
    expect(component.loadWarnings).toContain('Extracted records');
  });
});
