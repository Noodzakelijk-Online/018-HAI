import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IBrainCatalogResponse } from '../models/brain-catalog.model.interface'

@Injectable({ providedIn: 'root' })
export class BrainCatalogService {
  constructor(private http: HttpClient) {}

  overview(): Observable<IBrainCatalogResponse> {
    return this.http.get<IBrainCatalogResponse>('/api/v1/brain-catalog/')
  }
}
