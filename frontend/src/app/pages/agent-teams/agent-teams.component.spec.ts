import { AgentCoordinationMessage, AgentTeamContract } from '../../models/agent-teams.model'
import { AgentTeamsComponent } from './agent-teams.component'

describe('AgentTeamsComponent', () => {
  function component(): AgentTeamsComponent {
    return new AgentTeamsComponent({} as never, {} as never, {} as never, {} as never)
  }

  function selectedTeam(): AgentTeamContract {
    return {
      id: 'team-1', key: 'review-team', version: '1.0.0', name: 'Review team', purpose: 'Review evidence.',
      status: 'active', revision: 1, authorityCeiling: 1, riskCeiling: 'low',
      maximumDelegatedAuthority: 1, maximumDelegatedRisk: 'low', advisoryOnly: true,
      grantsExecutionAuthority: false, executionAuthorizationRequired: true,
      capabilities: [], roles: [], members: [], evidenceRefs: ['source:test'],
      consensus: {
        mode: 'majority', decisionPayloadSchema: 'hai.agent-team.decision.v1', quorum: 2,
        minimumSupport: 2, allowAbstention: true, requireEvidence: true,
        conflictEscalationRequired: true, tieOutcome: 'escalated',
      },
      provenance: {
        source: 'test', authoredBy: 'test', registeredBy: 'test', registeredAt: '', evidenceDigest: 'digest',
      },
      contractDigest: 'digest', createdAt: '', updatedAt: '',
    }
  }

  function decisionMessage(id: string, senderId: string): AgentCoordinationMessage {
    return {
      id, correlationId: 'correlation-1', idempotencyKey: id, schemaVersion: '1', type: 'decision',
      sender: { id: senderId, role: 'reviewer', authorityCeiling: 1 },
      recipient: { id: 'recipient-1', role: 'coordinator', authorityCeiling: 1 },
      confidentiality: 'internal', authorityLevel: 1,
      payload: { schema: 'hai.agent-team.decision.v1', subject: 'Accept the result?', data: { position: 'support' } },
      payloadDigest: 'digest', evidenceRefs: ['source:test'], requiresAck: true,
      createdAt: '', expiresAt: '', provenanceSummary: 'test',
    }
  }

  it('requires distinct voting members before promoting consensus evaluation', () => {
    const fixture = component()
    fixture.selected = selectedTeam()
    fixture.consensusForm = { correlationId: 'correlation-1', issue: 'Accept the result?' }
    const message = decisionMessage('message-1', 'member-1')

    fixture.messages = [message, { ...message, id: 'message-2' }]
    expect(fixture.consensusReady).toBeFalse()

    fixture.messages = [message, decisionMessage('message-3', 'member-2')]
    expect(fixture.consensusReady).toBeTrue()
  })

  it('does not promote consensus after an outcome already exists', () => {
    const fixture = component()
    fixture.selected = selectedTeam()
    fixture.consensusForm = { correlationId: 'correlation-1', issue: 'Accept the result?' }
    fixture.messages = [
      decisionMessage('message-1', 'member-1'),
      decisionMessage('message-2', 'member-2'),
    ]
    fixture.outcomes = [{ correlationId: 'correlation-1' } as never]

    expect(fixture.consensusReady).toBeFalse()
  })
})
