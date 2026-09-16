/**
 * 资产 API（ARCHITECTURE §5.7）。
 */
import { del, get, post, put } from './http'
import type { PageResult } from '@/types/common'
import type {
  Asset,
  AssetHistory,
  AssetQuery,
  AssetTransitionReq,
  SaveAssetReq
} from '@/types/asset'

/** GET /assets */
export function fetchAssets(query: AssetQuery): Promise<PageResult<Asset>> {
  return get<PageResult<Asset>>('/assets', query as unknown as Record<string, unknown>)
}

/** POST /assets */
export function createAsset(data: SaveAssetReq): Promise<Asset> {
  return post<Asset>('/assets', data)
}

/** GET /assets/:id */
export function fetchAsset(id: number): Promise<Asset> {
  return get<Asset>(`/assets/${id}`)
}

/** PUT /assets/:id */
export function updateAsset(id: number, data: SaveAssetReq): Promise<Asset> {
  return put<Asset>(`/assets/${id}`, data)
}

/** DELETE /assets/:id */
export function deleteAsset(id: number): Promise<null> {
  return del<null>(`/assets/${id}`)
}

/** POST /assets/:id/transition 生命周期流转 */
export function transitionAsset(id: number, data: AssetTransitionReq): Promise<Asset> {
  return post<Asset>(`/assets/${id}/transition`, data)
}

/** GET /assets/:id/history 生命周期历史 */
export function fetchAssetHistory(id: number): Promise<AssetHistory[]> {
  return get<AssetHistory[]>(`/assets/${id}/history`)
}

/** POST /assets/:id/bind-ci 绑定 CI（1:1） */
export function bindAssetCi(id: number, ciId: number): Promise<Asset> {
  return post<Asset>(`/assets/${id}/bind-ci`, { ci_id: ciId })
}

/** DELETE /assets/:id/bind-ci 解绑 */
export function unbindAssetCi(id: number): Promise<null> {
  return del<null>(`/assets/${id}/bind-ci`)
}
