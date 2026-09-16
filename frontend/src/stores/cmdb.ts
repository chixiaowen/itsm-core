/**
 * CMDB Store：CI / 关系 / 拓扑（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as cmdbApi from '@/api/cmdb'
import type {
  CI,
  CIRelation,
  CiQuery,
  CreateRelationReq,
  SaveCiReq,
  Topology
} from '@/types/cmdb'

export const useCmdbStore = defineStore('cmdb', () => {
  const cis = ref<CI[]>([])
  const relations = ref<CIRelation[]>([])
  const topology = ref<Topology>({ nodes: [], edges: [] })
  const current = ref<CI | null>(null)
  const ciTypes = ref<string[]>([])
  const total = ref(0)
  const loading = ref(false)
  const query = ref<CiQuery>({ page: 1, page_size: 20 })

  async function fetchCis(extra?: Partial<CiQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await cmdbApi.fetchCis(query.value)
      cis.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchDetail(id: number): Promise<CI> {
    loading.value = true
    try {
      const data = await cmdbApi.fetchCi(id)
      current.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  async function fetchCiTypes(): Promise<string[]> {
    ciTypes.value = await cmdbApi.fetchCiTypes()
    return ciTypes.value
  }

  async function saveCi(id: number | null, payload: SaveCiReq): Promise<CI> {
    const data = id ? await cmdbApi.updateCi(id, payload) : await cmdbApi.createCi(payload)
    current.value = data
    return data
  }

  async function addRelation(id: number, payload: CreateRelationReq): Promise<CIRelation> {
    const data = await cmdbApi.createRelation(id, payload)
    await fetchRelations(id)
    return data
  }

  async function removeRelation(id: number, relId: number): Promise<void> {
    await cmdbApi.deleteRelation(id, relId)
    await fetchRelations(id)
  }

  /** CI 关系无独立列表接口，这里从拓扑响应派生（仅用于展示） */
  async function fetchRelations(id: number): Promise<void> {
    const topo = await cmdbApi.fetchTopology(id)
    relations.value = topo.edges.map((edge, index) => ({
      id: index + 1,
      source_ci_id: edge.source,
      target_ci_id: edge.target,
      relation_type: edge.relation_type,
      created_at: ''
    }))
  }

  async function fetchTopology(id: number, depth = 2, direction = 'both'): Promise<Topology> {
    loading.value = true
    try {
      topology.value = await cmdbApi.fetchTopology(id, depth, direction)
      return topology.value
    } finally {
      loading.value = false
    }
  }

  async function remove(id: number): Promise<void> {
    await cmdbApi.deleteCi(id)
  }

  return {
    cis,
    relations,
    topology,
    current,
    ciTypes,
    total,
    loading,
    query,
    fetchCis,
    fetchDetail,
    fetchCiTypes,
    saveCi,
    addRelation,
    removeRelation,
    fetchRelations,
    fetchTopology,
    remove
  }
})
