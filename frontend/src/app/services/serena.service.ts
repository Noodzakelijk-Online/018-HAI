import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { ISerenaStatus } from '../models/serena.model.interface'

@Injectable({ providedIn: 'root' })
export class SerenaService {
  private readonly apiUrl = '/api/v1/serena'

  constructor(private http: HttpClient) {}

  status(): Observable<ISerenaStatus> {
    return this.http.get<ISerenaStatus>(`${this.apiUrl}/status`)
  }
}
