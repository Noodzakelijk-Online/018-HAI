import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IPresidioStatus } from '../models/presidio.model.interface'

@Injectable({ providedIn: 'root' })
export class PresidioService {
  constructor(private http: HttpClient) {}

  status(): Observable<IPresidioStatus> {
    return this.http.get<IPresidioStatus>('/api/v1/presidio/status')
  }
}
