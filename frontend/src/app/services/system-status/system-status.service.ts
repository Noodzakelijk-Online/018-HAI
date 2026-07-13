import { Injectable } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Observable, throwError } from 'rxjs';
import { catchError } from 'rxjs/operators';
import { ISystemStatusService } from './system-status.service.interface';
import { ISystemReadiness } from '../../models/system-status.model.interface';

@Injectable({
  providedIn: 'root',
})
export class SystemStatusService implements ISystemStatusService {
  // Root path, not /api/v1: the readiness probe lives beside /healthz so an
  // orchestrator can reach it without knowing the API version.
  private readonly readinessUrl = '/readyz';

  constructor(private http: HttpClient) {}

  readiness(): Observable<ISystemReadiness> {
    return this.http
      .get<ISystemReadiness>(this.readinessUrl, { observe: 'body' })
      .pipe(
        catchError((error: HttpErrorResponse) => {
          // A not_ready backend answers 503 with the full report. That is data,
          // not a failure: surfacing "database unreachable" is the entire point
          // of the page. Re-throw only when there is no usable body (a real
          // transport error, or the gateway bouncing us to login).
          const body = error.error as ISystemReadiness | undefined;
          if (body && typeof body === 'object' && Array.isArray(body.checks)) {
            return new Observable<ISystemReadiness>((subscriber) => {
              subscriber.next(body);
              subscriber.complete();
            });
          }
          return throwError(() => error);
        })
      );
  }
}
