/**
 * 资产 Store（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as assetApi from '@/api/asset'
import type {
  Asset,
  AssetHistory,
  AssetQuery,
  AssetTransitionReq,
  SaveAssetReq
} from '@/types/asset'

export const useAssetStore = defineStore('asset', () => {
  const list = ref<Asset[]>([])
  const current = ref<Asset | null>(null)
  const history = ref<AssetHistory[]>([])
  const total = ref(0)
  const loading = ref(false)
  const query = ref<AssetQuery>({ page: 1, page_size: 20 })

  async function fetchList(extra?: Partial<AssetQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await assetApi.fetchAssets(query.value)
      list.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchDetail(id: number): Promise<Asset> {
    loading.value = true
    try {
      const data = await assetApi.fetchAsset(id)
      current.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  async function create(payload: SaveAssetReq): Promise<Asset> {
    const data = await assetApi.createAsset(payload)
    current.value = data
    return data
  }

  async function update(id: number, payload: SaveAssetReq): Promise<Asset> {
    const data = await assetApi.updateAsset(id, payload)
    current.value = data
    return data
  }

  async function transition(id: number, payload: AssetTransitionReq): Promise<Asset> {
    const data = await assetApi.transitionAsset(id, payload)
    current.value = data
    return data
  }

  async function fetchHistory(id: number): Promise<AssetHistory[]> {
    history.value = await assetApi.fetchAssetHistory(id)
    return history.value
  }

  async function bindCi(id: number, ciId: number): Promise<Asset> {
    const data = await assetApi.bindAssetCi(id, ciId)
    current.value = data
    return data
  }

  async function unbindCi(id: number): Promise<void> {
    await assetApi.unbindAssetCi(id)
    await fetchDetail(id)
  }

  async function remove(id: number): Promise<void> {
    await assetApi.deleteAsset(id)
  }

  return {
    list,
    current,
    history,
    total,
    loading,
    query,
    fetchList,
    fetchDetail,
    create,
    update,
    transition,
    fetchHistory,
    bindCi,
    unbindCi,
    remove
  }
})
