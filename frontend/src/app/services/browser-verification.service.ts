import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IBrowserVerificationStatus } from '../models/browser-verification.model.interface'

@Injectable({ providedIn: 'root' })
export class BrowserVerificationService {
  constructor(private http: HttpClient) {}

  status(): Observable<IBrowserVerificationStatus> {
    return this.http.get<IBrowserVerificationStatus>('/api/v1/browser-verification/status')
  }
}
