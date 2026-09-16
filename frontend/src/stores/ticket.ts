/**
 * 工单 Store（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as ticketApi from '@/api/ticket'
import type {
  CreateTicketReq,
  RatingReq,
  Ticket,
  TicketCategoryNode,
  TicketQuery,
  TransitionReq,
  UpdateTicketReq
} from '@/types/ticket'

export const useTicketStore = defineStore('ticket', () => {
  const list = ref<Ticket[]>([])
  const current = ref<Ticket | null>(null)
  const categories = ref<TicketCategoryNode[]>([])
  const total = ref(0)
  const loading = ref(false)
  const query = ref<TicketQuery>({ page: 1, page_size: 20 })

  async function fetchList(extra?: Partial<TicketQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await ticketApi.fetchTickets(query.value)
      list.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchDetail(id: number): Promise<Ticket> {
    loading.value = true
    try {
      const data = await ticketApi.fetchTicket(id)
      current.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  async function fetchCategories(): Promise<TicketCategoryNode[]> {
    categories.value = await ticketApi.fetchTicketCategories()
    return categories.value
  }

  async function create(payload: CreateTicketReq): Promise<Ticket> {
    const data = await ticketApi.createTicket(payload)
    current.value = data
    return data
  }

  async function update(id: number, payload: UpdateTicketReq): Promise<Ticket> {
    const data = await ticketApi.updateTicket(id, payload)
    current.value = data
    return data
  }

  async function transition(id: number, payload: TransitionReq): Promise<Ticket> {
    const data = await ticketApi.transitionTicket(id, payload)
    current.value = data
    return data
  }

  async function assign(id: number, assigneeId: number): Promise<Ticket> {
    const data = await ticketApi.assignTicket(id, assigneeId)
    current.value = data
    return data
  }

  async function rate(id: number, payload: RatingReq): Promise<Ticket> {
    const data = await ticketApi.rateTicket(id, payload)
    current.value = data
    return data
  }

  async function addComment(id: number, content: string, isInternal: boolean): Promise<void> {
    await ticketApi.commentTicket(id, content, isInternal)
  }

  async function remove(id: number): Promise<void> {
    await ticketApi.deleteTicket(id)
  }

  return {
    list,
    current,
    categories,
    total,
    loading,
    query,
    fetchList,
    fetchDetail,
    fetchCategories,
    create,
    update,
    transition,
    assign,
    rate,
    addComment,
    remove
  }
})
