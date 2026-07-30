import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IMiniSWEStatus } from '../models/miniswe.model.interface'

@Injectable({ providedIn: 'root' })
export class MiniSWEService {
  constructor(private http: HttpClient) {}

  status(): Observable<IMiniSWEStatus> {
    return this.http.get<IMiniSWEStatus>('/api/v1/mini-swe/status')
  }
}
