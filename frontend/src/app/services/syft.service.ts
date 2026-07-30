import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { ISyftStatus } from '../models/syft.model.interface';

@Injectable({ providedIn: 'root' })
export class SyftService {
  constructor(private http: HttpClient) {}

  status(): Observable<ISyftStatus> {
    return this.http.get<ISyftStatus>('/api/v1/syft/status');
  }
}
