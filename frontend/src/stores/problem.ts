/**
 * 问题 Store（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as problemApi from '@/api/problem'
import type {
  CreateProblemReq,
  KnownErrorReq,
  Problem,
  ProblemQuery,
  ProblemTransitionReq,
  UpdateProblemReq
} from '@/types/problem'

export const useProblemStore = defineStore('problem', () => {
  const list = ref<Problem[]>([])
  const current = ref<Problem | null>(null)
  const total = ref(0)
  const loading = ref(false)
  const query = ref<ProblemQuery>({ page: 1, page_size: 20 })

  async function fetchList(extra?: Partial<ProblemQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await problemApi.fetchProblems(query.value)
      list.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchDetail(id: number): Promise<Problem> {
    loading.value = true
    try {
      const data = await problemApi.fetchProblem(id)
      current.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  async function create(payload: CreateProblemReq): Promise<Problem> {
    const data = await problemApi.createProblem(payload)
    current.value = data
    return data
  }

  async function rca(id: number, payload: UpdateProblemReq): Promise<Problem> {
    const data = await problemApi.updateProblem(id, payload)
    current.value = data
    return data
  }

  async function transition(id: number, payload: ProblemTransitionReq): Promise<Problem> {
    const data = await problemApi.transitionProblem(id, payload)
    current.value = data
    return data
  }

  async function markKnownError(id: number, payload: KnownErrorReq): Promise<Problem> {
    const data = await problemApi.markKnownError(id, payload)
    current.value = data
    return data
  }

  async function linkChanges(id: number, changeIds: number[]): Promise<void> {
    await problemApi.linkProblemChanges(id, changeIds)
    await fetchDetail(id)
  }

  async function remove(id: number): Promise<void> {
    await problemApi.deleteProblem(id)
  }

  return {
    list,
    current,
    total,
    loading,
    query,
    fetchList,
    fetchDetail,
    create,
    rca,
    transition,
    markKnownError,
    linkChanges,
    remove
  }
})
