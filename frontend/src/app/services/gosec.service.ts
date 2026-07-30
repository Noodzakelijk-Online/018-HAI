import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { IGosecStatus } from '../models/gosec.model.interface';

@Injectable({ providedIn: 'root' })
export class GosecService {
  constructor(private http: HttpClient) {}

  status(): Observable<IGosecStatus> {
    return this.http.get<IGosecStatus>('/api/v1/gosec/status');
  }
}
