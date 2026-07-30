import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import { IBrainCatalogAdoptionPlan, IBrainCatalogCapabilityRecommendationResponse, IBrainCatalogOSSInsightDiscoveryReport, IBrainCatalogOSSInsightReview, IBrainCatalogResponse, IBrainCatalogUpstreamReview } from '../models/brain-catalog.model.interface'

@Injectable({ providedIn: 'root' })
export class BrainCatalogService {
  constructor(private http: HttpClient) {}

  overview(): Observable<IBrainCatalogResponse> {
    return this.http.get<IBrainCatalogResponse>('/api/v1/brain-catalog/')
  }

  adoptionPlan(): Observable<IBrainCatalogAdoptionPlan> {
    return this.http.get<IBrainCatalogAdoptionPlan>('/api/v1/brain-catalog/adoption-plan')
  }

  revalidate(id: string): Observable<IBrainCatalogUpstreamReview> {
	return this.http.post<IBrainCatalogUpstreamReview>(`/api/v1/brain-catalog/${encodeURIComponent(id)}/revalidate`, {})
  }

  revalidateOSSInsightCollections(): Observable<IBrainCatalogOSSInsightReview> {
    return this.http.post<IBrainCatalogOSSInsightReview>('/api/v1/brain-catalog/ossinsight/revalidate', {})
  }

  discoverOSSInsightRepositories(): Observable<IBrainCatalogOSSInsightDiscoveryReport> {
    return this.http.post<IBrainCatalogOSSInsightDiscoveryReport>('/api/v1/brain-catalog/ossinsight/discover', {})
  }

  discoverReviewableOSSInsightRepositories(): Observable<IBrainCatalogOSSInsightDiscoveryReport> {
    return this.http.post<IBrainCatalogOSSInsightDiscoveryReport>('/api/v1/brain-catalog/ossinsight/discover/reviewable', {})
  }

  revalidateOSSInsightDiscovery(repository: string, scope: 'candidate' | 'reviewable' = 'candidate'): Observable<IBrainCatalogUpstreamReview> {
    return this.http.post<IBrainCatalogUpstreamReview>('/api/v1/brain-catalog/ossinsight/discoveries/revalidate', { repository, scope })
  }

  recommendCapabilities(need: string): Observable<IBrainCatalogCapabilityRecommendationResponse> {
    return this.http.post<IBrainCatalogCapabilityRecommendationResponse>('/api/v1/brain-catalog/recommend', { need })
  }
}
