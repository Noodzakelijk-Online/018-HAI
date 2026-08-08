import { HttpClientTestingModule } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { NzModalService } from 'ng-zorro-antd/modal';
import { TaskBlueprintModule } from './task-blueprint.module';

describe('TaskBlueprintModule', () => {
  it('provides the modal dependency required by the lazy route component', () => {
    TestBed.configureTestingModule({
      imports: [HttpClientTestingModule, NoopAnimationsModule, TaskBlueprintModule],
    });

    expect(TestBed.inject(NzModalService)).toBeTruthy();
  });
});
