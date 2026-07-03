import { Observable } from 'rxjs';
import {
  IAIConversationArchive,
  IAIConversationImportResult,
  ICommandDashboard,
  IMemoryEngineSearchResult,
} from '../models/memory-engine.model.interface';

export interface IMemoryEngineService {
  dashboard(): Observable<ICommandDashboard>;
  conversations(limit?: number): Observable<IAIConversationArchive[]>;
  importConversation(request: Record<string, unknown>): Observable<IAIConversationImportResult>;
  search(query: string, projectKey?: string, limit?: number): Observable<IMemoryEngineSearchResult>;
  deleteConversation(id: string): Observable<void>;
}
