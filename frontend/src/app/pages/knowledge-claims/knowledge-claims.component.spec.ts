import { of } from 'rxjs'
import { IClaimReviewQueue } from '../../models/knowledge-claim.model.interface'
import { AuthSessionService } from '../../services/auth-session.service'
import { KnowledgeClaimService } from '../../services/knowledge-claim.service'
import { KnowledgeClaimsComponent } from './knowledge-claims.component'

describe('KnowledgeClaimsComponent', () => {
  let claims: jasmine.SpyObj<KnowledgeClaimService>
  let auth: jasmine.SpyObj<AuthSessionService>
  let notification: jasmine.SpyObj<any>
  let router: jasmine.SpyObj<any>
  let component: KnowledgeClaimsComponent

  const queue: IClaimReviewQueue = {
    effectiveAt: '2026-08-04T10:00:00Z',
    observedBy: '2026-08-04T10:00:00Z',
    truncated: false,
    counts: { conflicting: 1 },
    items: [{
      claim: {
        id: 'claim-1', ownerIdentity: 'robert', workspaceId: '018-HAI',
        subject: 'HAI', predicate: 'status', object: 'ready',
        effectiveFrom: '2026-08-04T09:00:00Z', observedAt: '2026-08-04T09:00:00Z',
        verificationStatus: 'source_supported', provenance: [{
          referenceId: 'source-1', contentDigest: 'a'.repeat(64),
          capturedAt: '2026-08-04T09:00:00Z', localOnly: true,
        }],
        provenanceDigest: 'b'.repeat(64), sensitivity: 'internal', localOnly: true,
        claimDigest: 'c'.repeat(64), createdAt: '2026-08-04T09:00:00Z',
      },
      assessment: {
        claimId: 'claim-1', subject: 'HAI', predicate: 'status', object: 'ready',
        status: 'conflicting', effectiveAt: '2026-08-04T10:00:00Z', observedBy: '2026-08-04T10:00:00Z',
        reasons: ['another active claim has a different value'], evidenceIds: [], supportingClaimIds: [],
        conflictingClaimIds: ['claim-2'], supersedingClaimIds: [], truncated: false,
      },
    }],
  }

  beforeEach(() => {
    claims = jasmine.createSpyObj<KnowledgeClaimService>('KnowledgeClaimService', ['reviewQueue', 'lifecycle', 'correct'])
    auth = jasmine.createSpyObj<AuthSessionService>('AuthSessionService', ['session'])
    notification = jasmine.createSpyObj('NzNotificationService', ['success', 'warning', 'error'])
    router = jasmine.createSpyObj('Router', ['navigate'])
    claims.reviewQueue.and.returnValue(of(queue))
    claims.lifecycle.and.returnValue(of({ claim: queue.items[0].claim, supersedes: [], supersededBy: [], conflicts: [], truncated: false }))
    auth.session.and.returnValue(of({
      authenticated: true, subject: 'robert', role: 'owner',
      permissions: { canRead: true, canOperate: true, canApprove: true, canAdminister: true },
    }))
    const route: any = {
      snapshot: { queryParamMap: { get: () => null } },
    }
    component = new KnowledgeClaimsComponent(claims, auth, notification, route, router)
  })

  it('loads conflicts as the first operator attention queue', () => {
    component.ngOnInit()

    expect(component.loading).toBeFalse()
    expect(component.attentionItems.length).toBe(1)
    expect(component.visibleItems[0].claim.id).toBe('claim-1')
    expect(component.canApprove).toBeTrue()
  })

  it('opens immutable lifecycle detail rather than editing a claim in place', () => {
    component.ngOnInit()
    component.openInspector(queue.items[0])

    expect(claims.lifecycle).toHaveBeenCalledWith('018-HAI', 'claim-1')
    expect(component.lifecycle?.claim.id).toBe('claim-1')
    expect(component.inspectorOpen).toBeTrue()
  })
})
