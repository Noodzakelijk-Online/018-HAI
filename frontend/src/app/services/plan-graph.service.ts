import { HttpClient } from '@angular/common/http'
import { Injectable } from '@angular/core'
import { map, Observable } from 'rxjs'
import {
  IPlanAcceptRequest,
  IPlanBinding,
  IPlanGraph,
  IPlanGraphListResponse,
  IPlanPreviewRequest,
  IPlanReplanRequest,
  IPlanResourceEstimate,
  IPlanTransportBindings,
  IPlanTransportEdge,
  IPlanTransportNode,
  PlanApprovalStatus,
  PlanOwnerType,
  PlanRisk,
} from '../models/plan-graph.model.interface'

interface IPlanTransportRepair {
  reason: string
  trigger: string
  previousRevision: number
  previousDigest: string
  createdBy: string
  createdAt: string
}

interface IPlanTransport {
  id: string
  title: string
  status: 'draft' | 'accepted'
  revision: number
  digest: string
  parentRevision?: number
  parentDigest?: string
  nodes: IPlanTransportNode[]
  edges: IPlanTransportEdge[]
  repair?: IPlanTransportRepair
  createdBy: string
  createdAt: string
  acceptedAt?: string
  canExecute: boolean
}

@Injectable({ providedIn: 'root' })
export class PlanGraphService {
  private readonly apiUrl = '/api/v1/plans'

  constructor(private readonly http: HttpClient) {}

  list(): Observable<IPlanGraph[]> {
    return this.http.get<IPlanTransport[] | IPlanGraphListResponse>(this.apiUrl).pipe(
      map((response) => {
        const plans = Array.isArray(response) ? response : response?.plans as unknown as IPlanTransport[]
        return (plans || []).map((plan) => this.normalize(plan))
      }),
    )
  }

  get(id: string): Observable<IPlanGraph> {
    return this.http.get<IPlanTransport>(`${this.apiUrl}/${encodeURIComponent(id)}`).pipe(
      map((plan) => this.normalize(plan)),
    )
  }

  preview(request: IPlanPreviewRequest): Observable<IPlanGraph> {
    return this.http.post<IPlanTransport>(`${this.apiUrl}/preview`, request).pipe(
      map((plan) => this.normalize(plan)),
    )
  }

  accept(id: string, request: IPlanAcceptRequest): Observable<IPlanGraph> {
    return this.http.post<IPlanTransport>(`${this.apiUrl}/${encodeURIComponent(id)}/accept`, request).pipe(
      map((plan) => this.normalize(plan)),
    )
  }

  replan(id: string, request: IPlanReplanRequest): Observable<IPlanGraph> {
    return this.http.post<IPlanTransport>(`${this.apiUrl}/${encodeURIComponent(id)}/replan`, request).pipe(
      map((plan) => this.normalize(plan)),
    )
  }

  private normalize(plan: IPlanTransport | null | undefined): IPlanGraph {
    const source = plan || ({} as IPlanTransport)
    const edges = source.edges || []
    const nodes = (source.nodes || []).map((node, index) => this.normalizeNode(node, index, edges))
    const risks: PlanRisk[] = nodes.map((node) => node.risk)
    const approval = this.planApproval(nodes)
    const repair = source.repair
    const starts = nodes.map((node) => node.plannedStartAt).filter((value): value is string => !!value).sort()
    const deadlines = nodes.map((node) => node.plannedEndAt).filter((value): value is string => !!value).sort()
    return {
      id: source.id,
      title: source.title,
      objective: nodes.find((node) => node.transport.type === 'objective')?.title || source.title,
      status: source.status,
      risk: risks.includes('high') ? 'high' : risks.includes('medium') ? 'medium' : 'low',
      revision: source.revision,
      planDigest: source.digest,
      previousPlanId: undefined,
      successCriteria: nodes.filter((node) => node.transport.type === 'success_criterion').map((node) => node.title),
      nodes,
      edges: edges.map((edge) => ({
        id: edge.id,
        fromNodeId: edge.from,
        toNodeId: edge.to,
        type: this.edgeType(edge.type),
        required: true,
        lagMinutes: edge.lagMinutes,
      })),
      criticalPathNodeIds: [],
      constraints: [
        ...nodes.flatMap((node) => node.constraints),
        ...nodes.filter((node) => node.transport.type === 'constraint').map((node) => ({
          id: `${node.id}:constraint`,
          type: 'other' as const,
          label: node.title,
          hard: true,
          satisfied: node.status === 'completed',
        })),
      ],
      resourceEstimates: this.totalResources(source.nodes || []),
      bindings: this.uniqueBindings(nodes.flatMap((node) => node.bindings)),
      approval,
      frameworkSelectionDigest: this.sharedDigest(source.nodes || [], 'frameworkDigest'),
      evidenceDigest: this.sharedDigest(source.nodes || [], 'evidenceDigest'),
      contextSnapshotDigest: undefined,
      authorizationReceiptId: undefined,
      verificationId: undefined,
      plannedStartAt: starts[0],
      plannedEndAt: deadlines[deadlines.length - 1],
      deadlineAt: deadlines[deadlines.length - 1],
      createdAt: source.createdAt,
      updatedAt: source.createdAt,
      acceptedAt: source.acceptedAt,
      completedAt: undefined,
      revisions: [{
        id: `${source.id}:${source.revision}`,
        revision: source.revision,
        status: source.status,
        planDigest: source.digest,
        reason: repair?.reason,
        createdAt: source.createdAt,
        createdBy: source.createdBy,
      }],
      repairHistory: repair ? [{
        id: `${source.id}:repair:${source.revision}`,
        revision: source.revision,
        reason: repair.reason,
        summary: repair.trigger,
        actor: repair.createdBy,
        createdAt: repair.createdAt,
        previousPlanDigest: repair.previousDigest,
        resultingPlanDigest: source.digest,
      }] : [],
      canExecute: source.canExecute === true,
    }
  }

  private normalizeNode(node: IPlanTransportNode, index: number, edges: IPlanTransportEdge[]): IPlanGraph['nodes'][number] {
    const approvalStatus = this.approvalStatus(node.approvalState)
    const bindings = this.bindings(node.bindings || {})
    const constraints = []
    if (node.earliestStart) constraints.push({ id: `${node.id}:earliest`, type: 'earliest_start' as const, label: 'Earliest start', value: node.earliestStart, hard: true })
    if (node.deadline) constraints.push({ id: `${node.id}:deadline`, type: 'deadline' as const, label: 'Deadline', value: node.deadline, hard: true })
    const resources = []
    if (node.estimatedMinutes > 0) resources.push({ resourceType: 'time', label: 'Estimated time', amount: node.estimatedMinutes, unit: 'minutes', sourceLabel: 'plan node estimate' })
    if (node.estimatedCostEur > 0) resources.push({ resourceType: 'budget', label: 'Estimated paid cost', amount: node.estimatedCostEur, unit: 'EUR', sourceLabel: 'plan node estimate' })
    return {
      id: node.id,
      sequence: index + 1,
      title: node.title,
      status: node.status,
      ownerType: this.ownerType(node.owner),
      ownerLabel: node.owner,
      risk: node.risk,
      approvalRequired: node.approvalState === 'required' || node.status === 'needs_approval',
      approvalStatus,
      dependencyIds: edges.filter((edge) => edge.to === node.id).map((edge) => edge.from),
      plannedStartAt: node.earliestStart,
      plannedEndAt: node.deadline,
      estimatedDurationMinutes: node.estimatedMinutes,
      constraints,
      resourceEstimates: resources,
      bindings,
      frameworkSelectionDigest: node.frameworkDigest,
      evidenceDigest: node.evidenceDigest,
      transport: node,
    }
  }

  private planApproval(nodes: IPlanGraph['nodes']): IPlanGraph['approval'] {
    const required = nodes.some((node) => node.approvalRequired)
    if (!required) return { required: false, status: 'not_required' }
    const statuses = nodes.filter((node) => node.approvalRequired).map((node) => node.approvalStatus)
    const status: PlanApprovalStatus = statuses.includes('rejected') ? 'rejected' : statuses.every((value) => value === 'approved') ? 'approved' : 'pending'
    return { required: true, status, reason: 'One or more plan nodes require explicit approval.' }
  }

  private approvalStatus(value: IPlanTransportNode['approvalState']): PlanApprovalStatus {
    if (value === 'granted') return 'approved'
    if (value === 'rejected') return 'rejected'
    if (value === 'required') return 'pending'
    return 'not_required'
  }

  private ownerType(owner: string): PlanOwnerType {
    const value = (owner || '').toLowerCase()
    if (value === 'robert' || value.includes('owner')) return 'robert'
    if (value === 'hai' || value.includes('system')) return 'hai'
    if (value.includes('agent')) return 'agent'
    if (value.includes('external')) return 'external'
    return 'unassigned'
  }

  private bindings(value: IPlanTransportBindings): IPlanGraph['bindings'] {
    const bindings: IPlanBinding[] = []
    if (value.pursuitId) bindings.push({ type: 'pursuit', id: value.pursuitId })
    if (value.workflowId) bindings.push({ type: 'workflow', id: value.workflowId })
    if (value.taskId) bindings.push({ type: 'task', id: value.taskId })
    if (value.agentId) bindings.push({ type: 'agent', id: value.agentId })
    return bindings
  }

  private uniqueBindings(bindings: IPlanGraph['bindings']): IPlanGraph['bindings'] {
    const seen = new Set<string>()
    return bindings.filter((binding) => {
      const key = `${binding.type}:${binding.id}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
  }

  private totalResources(nodes: IPlanTransportNode[]): IPlanGraph['resourceEstimates'] {
    const minutes = nodes.reduce((sum, node) => sum + Math.max(0, node.estimatedMinutes || 0), 0)
    const cost = nodes.reduce((sum, node) => sum + Math.max(0, node.estimatedCostEur || 0), 0)
    const resources: IPlanResourceEstimate[] = []
    if (minutes) resources.push({ resourceType: 'time', label: 'Total estimated time', amount: minutes, unit: 'minutes', sourceLabel: 'sum of plan node estimates' })
    if (cost) resources.push({ resourceType: 'budget', label: 'Total estimated paid cost', amount: cost, unit: 'EUR', sourceLabel: 'sum of plan node estimates' })
    return resources
  }

  private sharedDigest(nodes: IPlanTransportNode[], key: 'frameworkDigest' | 'evidenceDigest'): string | undefined {
    const values = [...new Set(nodes.map((node) => node[key]).filter((value): value is string => !!value))]
    return values.length === 1 ? values[0] : undefined
  }

  private edgeType(value: string): IPlanGraph['edges'][number]['type'] {
    const supported = ['finish_to_start', 'start_to_start', 'finish_to_finish', 'conditional', 'information']
    return supported.includes(value) ? value as IPlanGraph['edges'][number]['type'] : 'other'
  }
}
