# Data Structure SDK（ds）

企业级泛型数据结构 SDK，为 bore 提供统一的数据抽象层。

> **核心理念**：数据怎么存 与 数据怎么算彻底分离 —— 算法只认接口，不认具体结构。

## 依赖铁律

```
      业务层（Workflow / StateMachine / ...）
          │
          ▼
Tree / Graph / Trie / Heap / Collection
          │
          ▼
       Core（接口定义）
          ▲
          │
    Algorithm（遍历：BFS / DFS）
```

- **Core**：只定义接口，零依赖。
- **Algorithm**：只依赖 Core + Internal，不碰 Tree / Graph / Trie。
- **数据结构**：实现 Core 中的接口。
- **Internal**：Queue / Stack 等基础组件，仅 ds/* 内部可见。

## 目录结构

```
ds/
├── core/              # ✅ 基础接口（10 个接口，100% 覆盖）
│   ├── collection.go  # Collection[T] — 根接口
│   ├── iterator.go    # Iterator[T] — 迭代器
│   ├── iterable.go    # Iterable[T] — 可迭代集合
│   ├── traversable.go # Traversable[T] — ★ 算法唯一依赖
│   ├── searchable.go  # Searchable[T] — 可搜索
│   ├── mutable.go     # Mutable[T] — 可修改
│   ├── cloneable.go   # Cloneable[T] — 可克隆
│   ├── comparable.go  # Comparable[T] — 可比较
│   ├── visitor.go     # Visitor[T] — 访问回调
│   ├── weighted.go    # Weighted — 有权重
│   ├── types.go       # Order 遍历顺序枚举
│   ├── errors.go      # 哨兵错误
│   └── option.go      # Option[T] 可选值
│
├── internal/          # ✅ 内部基础组件（98.4% 覆盖）
│   ├── queue.go       # Queue[T] — FIFO 环形缓冲队列
│   └── stack.go       # Stack[T] — LIFO 切片栈
│
├── algorithm/         # ✅ 算法层
│   └── traversal/     # BFS / DFS（100% 覆盖）
│       ├── bfs.go     # BFS — 广度优先遍历
│       └── dfs.go     # DFS — 深度优先遍历
│
├── tree/              # ✅ 二叉搜索树（93.4% 覆盖）
│   ├── bst.go         # BST[T] — 实现 7 个 core 接口
│   └── bst_iterator.go# 中序迭代器
│
├── graph/             # ✅ 图（85.9% 覆盖）
│   └── graph.go       # 有向/无向图，邻接表，加权边
│
├── trie/              # ✅ 前缀树（96.2% 覆盖）
│   ├── trie.go        # Trie[V] — 键值映射 + 前缀匹配
│   └── trie_addendum.go# Add/Put + Iterator
│
├── heap/              # ✅ 二叉堆（98.6% 覆盖）
│   └── heap.go        # MinHeap / MaxHeap 切片堆
│
├── collection/        # ✅ 通用集合（86.8% 覆盖）
│   ├── linkedlist.go  # LinkedList[T] — 双向链表
│   ├── set.go         # Set[T] — 哈希集合
│   └── deque.go       # Deque[T] — 双端队列
│
└── README.md
```

## 全量测试

```
$ go test ./ds/... -cover

ds/algorithm/traversal    100.0%
ds/collection              86.8%
ds/core                   100.0%
ds/graph                   85.9%
ds/heap                    98.6%
ds/internal                98.4%
ds/tree                    93.4%
ds/trie                    96.2%
────────────────────────────────
总计 8 个包，全部 PASS，平均覆盖率 ~94%
```

## 接口实现矩阵

每个数据结构实现了哪些 core 接口：

| 数据结构 | Collection | Iterable | Searchable | Mutable | Cloneable | Comparable | Traversable |
|----------|:----------:|:--------:|:----------:|:-------:|:---------:|:----------:|:-----------:|
| **BST** | ✅ | ✅ 中序 | ✅ O(log n) | ✅ | ✅ 深拷贝 | ✅ 结构相等 | ✅ 子节点 |
| **Graph** | ✅ | — | ✅ O(1) | ✅ | ✅ | — | ✅ 邻居 |
| **Trie** | ✅ | ✅ 字典序 | ✅ O(k) | ✅ Add/Put | — | — | — |
| **Heap** | ✅ | — | ✅ O(n) | ✅ | ✅ | — | — |
| **LinkedList** | ✅ | ✅ | — | ✅ | — | — | — |
| **Set** | ✅ | ✅ | ✅ O(1) | ✅ | ✅ | — | — |
| **Deque** | ✅ | — | — | ✅ | — | — | — |

## 算法复用验证

BST 和 Graph 实现了 `Traversable` → 直接使用 BFS/DFS 无需额外代码：

```go
bst := tree.NewBST[int]()
// ... 插入数据
traversal.BFS(bst, 5, func(v int) bool { fmt.Println(v); return true })

g := graph.NewDirected[int]()
// ... 添加顶点和边
traversal.DFS(g, 0, func(v int) bool { fmt.Println(v); return true })
```

## 新增数据结构的步骤

假设要新增 `Workflow`：

1. 让 `Workflow` 实现 `core.Traversable[T]`
2. 直接调用 `traversal.BFS(workflow, start, visit)`
3. ✅ 完成，零修改 algorithm / tree / graph

## 设计原则遵循

| 原则 | 遵守情况 |
|------|---------|
| SOLID（单一职责、开闭原则） | ✅ 每个接口 1-3 个方法；新增结构无需修改算法 |
| 接口隔离 | ✅ 10 个小型接口，按能力拆分 |
| 泛型全覆盖 | ✅ 零 `interface{}`、零反射、零 unsafe |
| 算法与结构解耦 | ✅ Algorithm 仅依赖 core.Traversable |
| 禁止循环依赖 | ✅ core ← internal ← algorithm/traversal/structure |
| GoDoc 中文注释 | ✅ 全部公开类型和方法 |

## 运行测试

```bash
cd bore

# 全部
go test ./ds/... -v -cover

# 单个
go test ./ds/tree/ -v -cover
```
