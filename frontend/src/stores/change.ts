/**
 * 变更 Store（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as changeApi from '@/api/change'
import type {
  ApprovalReq,
  Change,
  ChangeApproval,
  ChangeQuery,
  ChangeTransitionReq,
  CreateChangeReq,
  UpdateChangeReq
} from '@/types/change'

export const useChangeStore = defineStore('change', () => {
  const list = ref<Change[]>([])
  const current = ref<Change | null>(null)
  const approvals = ref<ChangeApproval[]>([])
  const total = ref(0)
  const loading = ref(false)
  const query = ref<ChangeQuery>({ page: 1, page_size: 20 })

  async function fetchList(extra?: Partial<ChangeQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await changeApi.fetchChanges(query.value)
      list.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchDetail(id: number): Promise<Change> {
    loading.value = true
    try {
      const data = await changeApi.fetchChange(id)
      current.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  async function create(payload: CreateChangeReq): Promise<Change> {
    const data = await changeApi.createChange(payload)
    current.value = data
    return data
  }

  async function update(id: number, payload: UpdateChangeReq): Promise<Change> {
    const data = await changeApi.updateChange(id, payload)
    current.value = data
    return data
  }

  async function transition(id: number, payload: ChangeTransitionReq): Promise<Change> {
    const data = await changeApi.transitionChange(id, payload)
    current.value = data
    return data
  }

  async function approve(id: number, payload: ApprovalReq): Promise<Change> {
    const data = await changeApi.approveChange(id, payload)
    current.value = data
    return data
  }

  async function fetchApprovals(id: number): Promise<ChangeApproval[]> {
    approvals.value = await changeApi.fetchChangeApprovals(id)
    return approvals.value
  }

  async function remove(id: number): Promise<void> {
    await changeApi.deleteChange(id)
  }

  return {
    list,
    current,
    approvals,
    total,
    loading,
    query,
    fetchList,
    fetchDetail,
    create,
    update,
    transition,
    approve,
    fetchApprovals,
    remove
  }
})
