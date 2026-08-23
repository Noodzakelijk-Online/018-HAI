import { FormBuilder } from '@angular/forms';
import { Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import { Subject } from 'rxjs';
import { IContextMemory } from '../../models/context-memory.model.interface';
import { MemoryComponent } from './memory.component';

describe('MemoryComponent', () => {
  function createComponent(): { component: MemoryComponent; memoryService: jasmine.SpyObj<any> } {
    const memoryService = jasmine.createSpyObj('ContextMemoryService', ['list']);
    const notification = jasmine.createSpyObj<NzNotificationService>('NzNotificationService', ['error']);
    const router = jasmine.createSpyObj<Router>('Router', ['navigate']);
    const component = new MemoryComponent(
      new FormBuilder(),
      memoryService,
      {} as any,
      notification,
      router,
      { mode: () => 'dark' } as any,
    );
    return { component, memoryService };
  }

  it('cancels an obsolete refresh so stale memories cannot replace the newest view', () => {
    const { component, memoryService } = createComponent();
    const first = new Subject<IContextMemory[]>();
    const second = new Subject<IContextMemory[]>();
    memoryService.list.and.returnValues(first.asObservable(), second.asObservable());

    component.refresh();
    component.refresh();

    second.next([{ id: 'new-memory', content: 'latest' } as IContextMemory]);
    second.complete();
    first.next([{ id: 'old-memory', content: 'stale' } as IContextMemory]);
    first.complete();

    expect(component.memories.map((memory) => memory.id)).toEqual(['new-memory']);
  });
});
