/**
 * 服务目录 Store（ARCHITECTURE §14.3）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as catalogApi from '@/api/catalog'
import type {
  OrderServiceReq,
  SaveServiceItemReq,
  ServiceCategoryNode,
  ServiceItem,
  ServiceItemQuery
} from '@/types/catalog'
import type { Ticket } from '@/types/ticket'

export const useCatalogStore = defineStore('catalog', () => {
  const categories = ref<ServiceCategoryNode[]>([])
  const portalCategories = ref<ServiceCategoryNode[]>([])
  const items = ref<ServiceItem[]>([])
  const portalItems = ref<ServiceItem[]>([])
  const current = ref<ServiceItem | null>(null)
  const total = ref(0)
  const loading = ref(false)
  const query = ref<ServiceItemQuery>({ page: 1, page_size: 20 })

  async function fetchTree(): Promise<ServiceCategoryNode[]> {
    categories.value = await catalogApi.fetchCategoryTree()
    return categories.value
  }

  async function fetchPortalCategories(): Promise<ServiceCategoryNode[]> {
    portalCategories.value = await catalogApi.fetchPortalCategories()
    return portalCategories.value
  }

  async function fetchItems(extra?: Partial<ServiceItemQuery>): Promise<void> {
    if (extra) query.value = { ...query.value, ...extra }
    loading.value = true
    try {
      const res = await catalogApi.fetchServiceItems(query.value)
      items.value = res.items
      total.value = res.total
      query.value.page = res.page
      query.value.page_size = res.page_size
    } finally {
      loading.value = false
    }
  }

  async function fetchPortalItems(params?: { category_id?: number; keyword?: string }): Promise<void> {
    loading.value = true
    try {
      portalItems.value = await catalogApi.fetchPortalItems(params)
    } finally {
      loading.value = false
    }
  }

  async function fetchItem(id: number): Promise<ServiceItem> {
    const data = await catalogApi.fetchServiceItem(id)
    current.value = data
    return data
  }

  async function saveItem(id: number | null, payload: SaveServiceItemReq): Promise<ServiceItem> {
    const data = id
      ? await catalogApi.updateServiceItem(id, payload)
      : await catalogApi.createServiceItem(payload)
    current.value = data
    return data
  }

  async function publish(id: number): Promise<ServiceItem> {
    const data = await catalogApi.publishServiceItem(id)
    current.value = data
    return data
  }

  async function offline(id: number): Promise<ServiceItem> {
    const data = await catalogApi.offlineServiceItem(id)
    current.value = data
    return data
  }

  async function archive(id: number): Promise<void> {
    await catalogApi.archiveServiceItem(id)
  }

  async function order(id: number, payload: OrderServiceReq): Promise<Ticket> {
    return catalogApi.orderServiceItem(id, payload)
  }

  async function saveCategory(id: number | null, payload: { name: string; parent_id: number | null; sort_order: number }): Promise<void> {
    if (id) await catalogApi.updateCategory(id, payload)
    else await catalogApi.createCategory(payload)
    await fetchTree()
  }

  async function removeCategory(id: number): Promise<void> {
    await catalogApi.deleteCategory(id)
    await fetchTree()
  }

  return {
    categories,
    portalCategories,
    items,
    portalItems,
    current,
    total,
    loading,
    query,
    fetchTree,
    fetchPortalCategories,
    fetchItems,
    fetchPortalItems,
    fetchItem,
    saveItem,
    publish,
    offline,
    archive,
    order,
    saveCategory,
    removeCategory
  }
})
