import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { Observable } from 'rxjs'
import {
  IAccountPermission,
  IBridgeContract,
  IFeedAudit,
  IFeedHealth,
  ISyncReport,
} from '../models/account-bridges.model.interface'

@Injectable({ providedIn: 'root' })
export class AccountBridgesService {
  private readonly apiUrl = '/api/v1/account-feeds'

  constructor(private http: HttpClient) {}

  bridges(): Observable<{ bridges: IBridgeContract[] }> {
    return this.http.get<{ bridges: IBridgeContract[] }>(`${this.apiUrl}/bridges`)
  }

  permissions(): Observable<{ permissions: IAccountPermission[] }> {
    return this.http.get<{ permissions: IAccountPermission[] }>(`${this.apiUrl}/permissions`)
  }

  feeds(): Observable<{ feeds: IFeedHealth[] }> {
    return this.http.get<{ feeds: IFeedHealth[] }>(this.apiUrl)
  }

  sync(id: string): Observable<ISyncReport> {
    return this.http.post<ISyncReport>(`${this.apiUrl}/${id}/sync`, {})
  }

  syncDue(): Observable<{ reports: ISyncReport[] }> {
    return this.http.post<{ reports: ISyncReport[] }>(`${this.apiUrl}/sync-due`, {})
  }

  audit(id: string): Observable<{ audit: IFeedAudit[] }> {
    return this.http.get<{ audit: IFeedAudit[] }>(`${this.apiUrl}/${id}/audit`)
  }
}
