import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  IAgentFrameworkMigrationPlan,
  IAutoGenCompatibilityEvent,
} from '../models/autogen-compat.model.interface';

@Injectable({ providedIn: 'root' })
export class AutoGenCompatibilityService {
  constructor(private http: HttpClient) {}

  microsoftAgentFrameworkMigrationPlan(
    workloadId: string,
    events: IAutoGenCompatibilityEvent[]
  ): Observable<IAgentFrameworkMigrationPlan> {
    return this.http.post<IAgentFrameworkMigrationPlan>('/api/v1/autogen-compat/migration-plan', {
      target: 'microsoft-agent-framework',
      workloadId,
      events,
    });
  }
}
