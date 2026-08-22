import { Observable } from 'rxjs';
import { ISystemInfo, ISystemReadiness } from '../../models/system-status.model.interface';

export interface ISystemStatusService {
  // readiness may resolve with an ISystemReadiness even on an HTTP 503: a
  // not_ready backend still returns the report body, and that body is exactly
  // what the operator needs to see. Only a transport failure is an error.
  readiness(): Observable<ISystemReadiness>;
  info(): Observable<ISystemInfo>;
}
