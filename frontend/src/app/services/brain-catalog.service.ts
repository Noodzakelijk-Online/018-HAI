import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IBrainCatalogResponse, IBrainCatalogUpstreamReview } from '../models/brain-catalog.model.interface'

@Injectable({ providedIn: 'root' })
export class BrainCatalogService {
  constructor(private http: HttpClient) {}

  overview(): Observable<IBrainCatalogResponse> {
    return this.http.get<IBrainCatalogResponse>('/api/v1/brain-catalog/')
  }

  revalidate(id: string): Observable<IBrainCatalogUpstreamReview> {
    return this.http.post<IBrainCatalogUpstreamReview>(`/api/v1/brain-catalog/${encodeURIComponent(id)}/revalidate`, {})
  }
}
