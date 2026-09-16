<template>
  <div class="ci-topology">
    <div v-if="!nodes.length" class="topo-empty">
      <el-empty description="暂无拓扑数据" />
    </div>
    <template v-else>
      <svg :viewBox="`0 0 ${width} ${height}`" class="topo-svg" role="img" aria-label="CI 拓扑图">
        <!-- 关系边 -->
        <g>
          <line
            v-for="(edge, idx) in edges"
            :key="`e-${idx}`"
            :x1="edge.x1"
            :y1="edge.y1"
            :x2="edge.x2"
            :y2="edge.y2"
            :stroke="EDGE_COLORS[edge.relation_type]"
            stroke-width="1.5"
            marker-end="url(#arrow)"
          />
        </g>
        <defs>
          <marker id="arrow" markerWidth="8" markerHeight="8" refX="18" refY="4" orient="auto">
            <path d="M0,0 L8,4 L0,8 Z" :fill="'#909399'" />
          </marker>
        </defs>
        <!-- 节点 -->
        <g
          v-for="node in layoutNodes"
          :key="node.id"
          class="topo-node"
          @click="emit('node-click', node.id)"
        >
          <circle
            :cx="node.x"
            :cy="node.y"
            r="26"
            :fill="NODE_COLORS[node.ci_type]"
            stroke="#fff"
            stroke-width="2"
          />
          <text :x="node.x" :y="node.y + 4" text-anchor="middle" class="node-code">
            {{ node.code.slice(0, 6) }}
          </text>
          <text :x="node.x" :y="node.y + 44" text-anchor="middle" class="node-name">
            {{ node.name.length > 8 ? node.name.slice(0, 8) + '…' : node.name }}
          </text>
        </g>
      </svg>
      <div class="topo-legend">
        <span v-for="(color, type) in NODE_COLORS" :key="type" class="legend-item">
          <i class="legend-dot" :style="{ backgroundColor: color }"></i>{{ typeLabel(type) }}
        </span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CI_TYPE_LABELS, type CiType, type RelationType, type Topology } from '@/types/cmdb'

const props = withDefaults(
  defineProps<{
    topology: Topology
    width?: number
    height?: number
  }>(),
  { width: 680, height: 420 }
)

const emit = defineEmits<{ (e: 'node-click', id: number): void }>()

const NODE_COLORS: Record<CiType, string> = {
  server: '#409eff',
  network: '#67c23a',
  database: '#e6a23c',
  application: '#9254de',
  terminal: '#909399',
  other: '#606266'
}

const EDGE_COLORS: Record<RelationType, string> = {
  depends_on: '#409eff',
  contains: '#67c23a',
  connects_to: '#909399'
}

function typeLabel(type: string): string {
  return CI_TYPE_LABELS[type as CiType] ?? type
}

const width = computed<number>(() => props.width)
const height = computed<number>(() => props.height)
const nodes = computed(() => props.topology.nodes)

interface PositionedNode {
  id: number
  code: string
  name: string
  ci_type: CiType
  x: number
  y: number
}

/** 纯 SVG 自绘：按环形布局定位节点（避免引入重型图库） */
const layoutNodes = computed<PositionedNode[]>(() => {
  const list = nodes.value
  const count = list.length
  if (count === 0) return []
  const cx = width.value / 2
  const cy = height.value / 2
  const radius = Math.max(80, Math.min(width.value, height.value) / 2 - 60)
  return list.map((node, index) => {
    if (count === 1) {
      return { ...node, x: cx, y: cy }
    }
    const angle = (2 * Math.PI * index) / count - Math.PI / 2
    return {
      ...node,
      x: Math.round(cx + radius * Math.cos(angle)),
      y: Math.round(cy + radius * Math.sin(angle))
    }
  })
})

/** 计算边的端点坐标 */
const edges = computed(() => {
  const map = new Map<number, PositionedNode>()
  layoutNodes.value.forEach((node) => map.set(node.id, node))
  return props.topology.edges
    .map((edge) => {
      const source = map.get(edge.source)
      const target = map.get(edge.target)
      if (!source || !target) return null
      return {
        x1: source.x,
        y1: source.y,
        x2: target.x,
        y2: target.y,
        relation_type: edge.relation_type
      }
    })
    .filter((item): item is NonNullable<typeof item> => item !== null)
})
</script>

<style scoped>
.ci-topology {
  width: 100%;
}
.topo-svg {
  width: 100%;
  height: auto;
  background-color: #fafcff;
  border: 1px solid #ebeef5;
  border-radius: 6px;
}
.topo-node {
  cursor: pointer;
}
.topo-node:hover circle {
  stroke: #303133;
}
.node-code {
  fill: #fff;
  font-size: 11px;
  pointer-events: none;
}
.node-name {
  fill: #303133;
  font-size: 12px;
  pointer-events: none;
}
.topo-legend {
  margin-top: 8px;
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
}
.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #606266;
}
.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}
.topo-empty {
  padding: 24px 0;
}
</style>
