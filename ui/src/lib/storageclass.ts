import { StorageClass } from 'kubernetes-types/storage/v1'

const defaultClassAnnotations = [
  'storageclass.kubernetes.io/is-default-class',
  'storageclass.beta.kubernetes.io/is-default-class',
]

export function isDefaultStorageClass(storageClass: StorageClass) {
  const annotations = storageClass.metadata?.annotations || {}
  return defaultClassAnnotations.some(
    (annotation) => annotations[annotation]?.toLowerCase() === 'true'
  )
}

export function getStorageClassParameters(storageClass: StorageClass) {
  return Object.entries(storageClass.parameters || {}).map(
    ([key, value]) => `${key}=${value}`
  )
}

export function getStorageClassTopologyRules(storageClass: StorageClass) {
  return (storageClass.allowedTopologies || []).flatMap((topology) =>
    (topology.matchLabelExpressions || []).map(
      (expression) =>
        `${expression.key}: ${(expression.values || []).join(', ')}`
    )
  )
}
