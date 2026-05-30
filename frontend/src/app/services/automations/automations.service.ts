import {Injectable} from '@angular/core';
import {IAutomationsService} from '../automations.service.interface';
import {
  IAutomationDiagnostics,
  IAutomationHealthResult,
  IAutomationHealthSummary,
  IAutomationModel,
} from "../../models/automation.model.interface";
import {Observable} from "rxjs";
import {HttpClient} from "@angular/common/http";

@Injectable({
  providedIn: 'root'
})
export class AutomationsService implements IAutomationsService {
  private apiUrl = '/api/v1/automation';

  constructor(private http: HttpClient) {
  }

  getAutomations(): Observable<IAutomationModel[]> {
    return this.http.get<IAutomationModel[]>(`${this.apiUrl}/`);
  }

  addAutomation(automation: IAutomationModel): Observable<IAutomationModel> {
    const formData = new FormData();

    formData.append('name', automation.name);
    formData.append('host', automation.host);
    formData.append('port', automation.port.toString());
    formData.append('position', automation.position.toString());
    formData.append('removeImage', automation.removeImage.toString());

    if (automation.id) {
      formData.append('id', automation.id);
    }

    if (automation.imageFile) {
      console.log(`Appending image file: ${automation.imageFile.name} with size: ${automation.imageFile.size} bytes`);
      formData.append('imageFile', automation.imageFile, automation.imageFile.name);
    }

    return this.http.post<IAutomationModel>(`${this.apiUrl}/`, formData);
  }


  deleteAutomation(id: string): Observable<void> {
    return this.http.delete<void>(`${this.apiUrl}/${id}`);
  }

  getAutomation(id: string): Observable<IAutomationModel> {
    return this.http.get<IAutomationModel>(`${this.apiUrl}/${id}`);
  }

  updateAutomation(automation: IAutomationModel): Observable<IAutomationModel> {
    console.log('sending: ', automation)
    return this.http.patch<IAutomationModel>(`${this.apiUrl}/`, automation);
  }

  swapAutomations(automation_id1: string, automation_id2: string): Observable<void> {
    return this.http.get<void>(`${this.apiUrl}/swap/${automation_id1}/${automation_id2}`);
  }

  getHealthSummary(): Observable<IAutomationHealthSummary> {
    return this.http.get<IAutomationHealthSummary>(`${this.apiUrl}/health/summary`);
  }

  runHealthCheck(id: string): Observable<IAutomationHealthResult> {
    return this.http.post<IAutomationHealthResult>(`${this.apiUrl}/${id}/health-check`, {});
  }

  getDiagnostics(id: string): Observable<IAutomationDiagnostics> {
    return this.http.get<IAutomationDiagnostics>(`${this.apiUrl}/${id}/diagnostics`);
  }

}
