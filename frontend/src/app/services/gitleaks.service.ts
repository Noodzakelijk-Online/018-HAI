import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { IGitleaksStatus } from '../models/gitleaks.model.interface';

@Injectable({ providedIn: 'root' })
export class GitleaksService {
  constructor(private http: HttpClient) {}

  status(): Observable<IGitleaksStatus> {
    return this.http.get<IGitleaksStatus>('/api/v1/gitleaks/status');
  }
}
