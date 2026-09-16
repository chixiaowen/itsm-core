<template>
  <div v-loading="loading" class="page-container">
    <el-page-header :content="ci?.code || 'CI 详情'" @back="router.back()" />

    <el-card v-if="ci" shadow="never" class="mt-16">
      <div class="detail-header">
        <span class="detail-title">{{ ci.name }}</span>
        <StatusTag :status="ci.status" />
        <el-tag type="info" size="small">{{ CI_TYPE_LABELS[ci.ci_type] }}</el-tag>
        <div class="header-actions">
          <el-button size="small" @click="router.push(`/cmdb/cis/${ci.id}/edit`)">编辑</el-button>
          <el-button type="success" size="small" @click="router.push(`/cmdb/cis/${ci.id}/topology`)">
            查看拓扑
          </el-button>
          <el-button type="primary" size="small" @click="relationDialog = true">新增关系</el-button>
        </div>
      </div>
    </el-card>

    <el-row v-if="ci" :gutter="16" class="mt-16">
      <el-col :xs="24" :md="10">
        <el-card shadow="never">
          <template #header><span class="card-title">基本信息</span></template>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="编码">{{ ci.code }}</el-descriptions-item>
            <el-descriptions-item label="名称">{{ ci.name }}</el-descriptions-item>
            <el-descriptions-item label="类型">{{ CI_TYPE_LABELS[ci.ci_type] }}</el-descriptions-item>
            <el-descriptions-item label="状态"><StatusTag :status="ci.status" /></el-descriptions-item>
            <el-descriptions-item label="负责人">{{ ci.owner_id ?? '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(ci.created_at) }}</el-descriptions-item>
          </el-descriptions>

          <div class="card-title mt-16">自定义属性</div>
          <el-descriptions v-if="attrEntries.length" :column="1" border>
            <el-descriptions-item v-for="attr in attrEntries" :key="attr.key" :label="attr.key">
              {{ attr.value }}
            </el-descriptions-item>
          </el-descriptions>
          <span v-else class="text-muted">无自定义属性</span>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="14">
        <el-card shadow="never">
          <template #header><span class="card-title">关系列表</span></template>
          <el-table :data="relationRows" size="small">
            <el-table-column prop="direction" label="方向" width="90" />
            <el-table-column label="类型" width="100">
              <template #default="{ row }">{{ RELATION_TYPE_LABELS[row.relation_type as RelationType] }}</template>
            </el-table-column>
            <el-table-column prop="name" label="对象" />
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-button text type="danger" @click="removeRelation(row as RelationRow)">解除</el-button>
              </template>
            </el-table-column>
            <template #empty><el-empty description="暂无关系" /></template>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <el-card v-if="ci" shadow="never" class="mt-16">
      <template #header><span class="card-title">变更记录</span></template>
      <Timeline :items="timelineItems" />
    </el-card>

    <el-dialog v-model="relationDialog" title="新增关系" width="460px">
      <el-form label-width="90px">
        <el-form-item label="目标 CI">
          <el-input-number v-model="relationForm.target_ci_id" :min="1" :controls="false" placeholder="目标 CI ID" style="width: 100%" />
        </el-form-item>
        <el-form-item label="关系类型">
          <el-select v-model="relationForm.relation_type" style="width: 100%">
            <el-option v-for="opt in RELATION_TYPE_OPTIONS" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="relationDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitRelation">确认</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import StatusTag from '@/components/StatusTag.vue'
import Timeline from '@/components/Timeline.vue'
import { useCmdbStore } from '@/stores/cmdb'
import { useAuditTimeline } from '@/composables/useAuditTimeline'
import { formatDateTime, sortTimeline, type TimelineItem } from '@/types/common'
import {
  CI_TYPE_LABELS,
  RELATION_TYPE_LABELS,
  RELATION_TYPE_OPTIONS,
  type RelationType,
  type Topology
} from '@/types/cmdb'

interface RelationRow {
  direction: string
  relation_type: RelationType
  name: string
  targetId: number
}

const route = useRoute()
const router = useRouter()
const store = useCmdbStore()

const ciId = computed(() => Number(route.params.id))
const ci = computed(() => store.current)
const loading = computed(() => store.loading)

/** 变更记录时间线（审计日志，entity_type=ci）：403 时优雅降级隐藏 */
const audit = useAuditTimeline({
  entityType: computed(() => 'ci'),
  entityId: computed(() => ciId.value || null)
})

const timelineItems = computed<TimelineItem[]>(() =>
  audit.forbidden.value ? [] : sortTimeline(audit.items.value)
)

const topology = ref<Topology>({ nodes: [], edges: [] })
const relationDialog = ref(false)
const submitting = ref(false)
const relationForm = reactive<{ target_ci_id: number; relation_type: RelationType }>({
  target_ci_id: 1,
  relation_type: 'depends_on'
})

const attrEntries = computed<{ key: string; value: string }[]>(() => {
  const raw = ci.value?.attrs
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    return Object.entries(parsed).map(([key, value]) => ({ key, value: String(value) }))
  } catch {
    return []
  }
})

const relationRows = computed<RelationRow[]>(() => {
  const nodeName = (id: number): string => {
    const node = topology.value.nodes.find((item) => item.id === id)
    return node ? `${node.code} ${node.name}` : `CI #${id}`
  }
  return topology.value.edges.map((edge) => {
    const outgoing = edge.source === ciId.value
    const otherId = outgoing ? edge.target : edge.source
    return {
      direction: outgoing ? '下游' : '上游',
      relation_type: edge.relation_type,
      name: nodeName(otherId),
      targetId: otherId
    }
  })
})

async function loadAll(): Promise<void> {
  await store.fetchDetail(ciId.value)
  try {
    topology.value = await store.fetchTopology(ciId.value)
  } catch {
    topology.value = { nodes: [], edges: [] }
  }
  await audit.reload()
}

async function submitRelation(): Promise<void> {
  submitting.value = true
  try {
    await store.addRelation(ciId.value, { ...relationForm })
    ElMessage.success('关系已新增')
    relationDialog.value = false
    await loadAll()
  } catch {
    /* 400 自环 / 409 重复 由拦截器提示 */
  } finally {
    submitting.value = false
  }
}

async function removeRelation(row: RelationRow): Promise<void> {
  try {
    await store.removeRelation(ciId.value, row.targetId)
    ElMessage.success('已解除')
    await loadAll()
  } catch {
    /* handled by interceptor */
  }
}

onMounted(() => {
  void loadAll()
})
</script>

<style scoped>
.header-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}
</style>
