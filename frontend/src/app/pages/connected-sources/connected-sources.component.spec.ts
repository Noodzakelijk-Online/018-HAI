import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { EMPTY } from 'rxjs';
import { ISourcePursuitRoutingOutcome } from '../../models/connected-source.model.interface';
import { ConnectedSourcesComponent } from './connected-sources.component';

describe('ConnectedSourcesComponent pursuit handoff', () => {
  function createComponent(sourceService: Record<string, unknown> = {}): { component: ConnectedSourcesComponent; router: jasmine.SpyObj<Router> } {
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    return {
      component: new ConnectedSourcesComponent(
        new FormBuilder(),
        sourceService as any,
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

  it('keeps the candidate graph readable when a source reference is absent', () => {
    const { component } = createComponent();

    expect(component.knowledgeGraphSourceLabel([])).toBe('source linked');
    expect(component.knowledgeGraphSourceLabel([{ extractionId: 'x', sourceLabel: 'Lawyer email' }])).toBe('Lawyer email');
  });

  it('routes a Docling source through controlled document extraction instead of generic sync', () => {
    const extractDocuments = jasmine.createSpy('extractDocuments').and.returnValue(EMPTY);
    const { component } = createComponent({ extractDocuments });

    component.syncSource({ id: 'source-1', connectorKey: 'docling-documents' } as any);

    expect(extractDocuments).toHaveBeenCalledOnceWith('source-1');
    expect(component.sourceActionLabel({ connectorKey: 'docling-documents' } as any)).toBe('Extract documents');
    expect(component.sourceActionTitle({ connectorKey: 'docling-documents' } as any)).toContain('explicit local document folder');
  });
});
