import { FormBuilder } from '@angular/forms'
import { convertToParamMap } from '@angular/router'
import { of } from 'rxjs'
import { GroundedAnswersComponent } from './grounded-answers.component'

describe('GroundedAnswersComponent RAGFlow evidence boundary', () => {
  function createComponent() {
    const verificationService = jasmine.createSpyObj('IVerificationService', ['answer', 'runs', 'runDetails'])
    const researchService = jasmine.createSpyObj('ResearchService', ['status', 'search'])
    const ragflowService = jasmine.createSpyObj('RAGFlowService', ['status', 'retrieve'])
    const notification = jasmine.createSpyObj('NzNotificationService', ['success', 'error', 'warning', 'info'])
    const route = { snapshot: { queryParamMap: convertToParamMap({}) } }
    const router = jasmine.createSpyObj('Router', ['navigate'])
    verificationService.runs.and.returnValue(of([]))
    verificationService.answer.and.returnValue(of({ run: { id: 'run-1', status: 'source_supported' }, claims: [], evidence: [], unsupportedClaims: [], researchQuestions: [], logs: [] }))
    researchService.status.and.returnValue(of({ configured: false, provider: 'SearXNG', scope: 'disabled' }))
    ragflowService.status.and.returnValue(of({ enabled: true, configured: true, provider: 'RAGFlow', datasetCount: 1, capabilities: [], restrictions: [], scope: 'candidate evidence only' }))

    return {
      component: new GroundedAnswersComponent(new FormBuilder(), verificationService, researchService, ragflowService, notification, route as any, router),
      verificationService,
      researchService,
      ragflowService,
      notification,
    }
  }

  it('reads RAGFlow configuration without retrieving evidence', () => {
    const { component, ragflowService } = createComponent()

    component.ngOnInit()

    expect(ragflowService.status).toHaveBeenCalled()
    expect(ragflowService.retrieve).not.toHaveBeenCalled()
    expect(component.ragflowStatus?.configured).toBeTrue()
  })

  it('attaches a selected RAGFlow chunk as unverified candidate evidence', () => {
    const { component, verificationService } = createComponent()
    component.useRAGFlowResult({
      chunkId: 'chunk 1',
      datasetId: 'legal-files',
      documentId: 'doc 1',
      documentName: 'Letter.pdf',
      content: 'The hearing is scheduled for 9 September.',
    })

    component.answer()

    expect(verificationService.answer).toHaveBeenCalledWith(jasmine.objectContaining({
      externalEvidence: [jasmine.objectContaining({
        sourceType: 'ragflow_candidate_evidence',
        sourceUri: 'ragflow://dataset/legal-files/document/doc%201/chunk/chunk%201',
        snippet: 'The hearing is scheduled for 9 September.',
        official: false,
        primary: false,
      })],
    }))
  })

  it('does not retain a RAGFlow selection after an explicit public-source selection', () => {
    const { component, verificationService } = createComponent()
    component.useRAGFlowResult({ chunkId: 'chunk-1', datasetId: 'documents', content: 'Local candidate.' })
    component.useResearchResult({ title: 'Official result', sourceUri: 'https://example.test/evidence', snippet: 'Public candidate.' })

    component.answer()

    expect(verificationService.answer).toHaveBeenCalledWith(jasmine.objectContaining({
      externalEvidence: [jasmine.objectContaining({ sourceType: 'local_research' })],
    }))
  })
})
