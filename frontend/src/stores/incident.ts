/**
 * 事件 Store（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as incidentApi from '@/api/incident'
import type {
  CreateIncidentReq,
  EscalateReq,
  Incident,
  IncidentQuery,
  IncidentTransitionReq,
  PriorityMatrixCell,
  UpdateIncidentReq
} from '@/types/incident'
import type { Priority } from '@/types/common'
import type { Ticket } from '@/types/ticket'

export const useIncidentStore = defineStore('incident', () => {
  const list = ref<Incident[]>([])
  const current = ref<Incident | null>(null)
  const matrix = ref<PriorityMatrixCell[]>([])
  const total = ref(0)
  const loading = ref(false)
  const query = ref<IncidentQuery>({ page: 1, page_size: 20 })

  async function fetchList(extra?: Partial<IncidentQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await incidentApi.fetchIncidents(query.value)
      list.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchDetail(id: number): Promise<Incident> {
    loading.value = true
    try {
      const data = await incidentApi.fetchIncident(id)
      current.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  async function fetchMatrix(): Promise<PriorityMatrixCell[]> {
    matrix.value = await incidentApi.fetchPriorityMatrix()
    return matrix.value
  }

  async function report(payload: CreateIncidentReq): Promise<Incident> {
    const data = await incidentApi.reportIncident(payload)
    current.value = data
    return data
  }

  async function update(id: number, payload: UpdateIncidentReq): Promise<Incident> {
    const data = await incidentApi.updateIncident(id, payload)
    current.value = data
    return data
  }

  async function transition(id: number, payload: IncidentTransitionReq): Promise<Incident> {
    const data = await incidentApi.transitionIncident(id, payload)
    current.value = data
    return data
  }

  async function overridePriority(id: number, priority: Priority): Promise<Incident> {
    const data = await incidentApi.overrideIncidentPriority(id, priority)
    current.value = data
    return data
  }

  async function escalate(id: number, payload: EscalateReq): Promise<Incident> {
    const data = await incidentApi.escalateIncident(id, payload)
    current.value = data
    return data
  }

  async function convertToTicket(id: number, title?: string): Promise<Ticket> {
    return incidentApi.convertIncidentToTicket(id, title)
  }

  async function linkTicket(id: number, ticketId: number): Promise<Incident> {
    const data = await incidentApi.linkIncidentTicket(id, ticketId)
    current.value = data
    return data
  }

  async function remove(id: number): Promise<void> {
    await incidentApi.deleteIncident(id)
  }

  return {
    list,
    current,
    matrix,
    total,
    loading,
    query,
    fetchList,
    fetchDetail,
    fetchMatrix,
    report,
    update,
    transition,
    overridePriority,
    escalate,
    convertToTicket,
    linkTicket,
    remove
  }
})
