<template>
  <div class="page-container">
    <el-card shadow="never">
      <div class="toolbar">
        <el-input-number v-model="ciId" :min="1" :controls="false" placeholder="起始 CI ID" style="width: 180px" />
        <el-select v-model="depth" style="width: 140px">
          <el-option :value="1" label="展开 1 层" />
          <el-option :value="2" label="展开 2 层" />
          <el-option :value="3" label="展开 3 层" />
        </el-select>
        <el-select v-model="direction" style="width: 140px">
          <el-option value="both" label="双向" />
          <el-option value="up" label="仅上游" />
          <el-option value="down" label="仅下游" />
        </el-select>
        <el-button type="primary" :loading="loading" @click="load">加载拓扑</el-button>
      </div>

      <div v-loading="loading" class="mt-16">
        <CiTopology :topology="store.topology" :height="460" @node-click="onNodeClick" />
        <div class="text-muted mt-8">提示：点击节点可进入该 CI 详情。</div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CiTopology from '@/components/CiTopology.vue'
import { useCmdbStore } from '@/stores/cmdb'

const route = useRoute()
const router = useRouter()
const store = useCmdbStore()

const ciId = ref<number>(route.params.id ? Number(route.params.id) : 1)
const depth = ref<number>(2)
const direction = ref<string>('both')
const loading = ref(false)

async function load(): Promise<void> {
  if (!ciId.value) return
  loading.value = true
  try {
    await store.fetchTopology(ciId.value, depth.value, direction.value)
  } catch {
    /* handled by interceptor */
  } finally {
    loading.value = false
  }
}

function onNodeClick(id: number): void {
  void router.push(`/cmdb/cis/${id}`)
}

onMounted(() => {
  void load()
})
</script>

<style scoped>
.toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}
</style>
