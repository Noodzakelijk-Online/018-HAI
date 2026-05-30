import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IVerificationService } from '../verification.service.interface';
import {
  IAnswerRequest,
  IVerificationResult,
  IVerificationRun,
} from '../../models/verification.model.interface';

@Injectable({
  providedIn: 'root',
})
export class VerificationService implements IVerificationService {
  private apiUrl = '/api/v1/verification';

  constructor(private http: HttpClient) {}

  answer(request: IAnswerRequest): Observable<IVerificationResult> {
    return this.http.post<IVerificationResult>(`${this.apiUrl}/answer`, request);
  }

  runs(): Observable<IVerificationRun[]> {
    return this.http.get<IVerificationRun[]>(`${this.apiUrl}/runs`);
  }

  runDetails(id: string): Observable<IVerificationResult> {
    return this.http.get<IVerificationResult>(`${this.apiUrl}/runs/${id}`);
  }
}
