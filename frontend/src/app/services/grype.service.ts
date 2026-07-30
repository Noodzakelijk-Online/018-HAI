import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { IGrypeStatus } from '../models/grype.model.interface';

@Injectable({ providedIn: 'root' })
export class GrypeService {
  constructor(private http: HttpClient) {}

  status(): Observable<IGrypeStatus> {
    return this.http.get<IGrypeStatus>('/api/v1/grype/status');
  }
}
