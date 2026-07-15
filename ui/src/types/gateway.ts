import { ObjectMeta } from 'kubernetes-types/meta/v1'

export interface Gateway {
  /** APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources */
  apiVersion?: 'gateway.networking.k8s.io/v1'
  /** Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds */
  kind?: 'Gateway'
  /** Standard object's metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata */
  metadata?: ObjectMeta
  /** spec is the desired state of the Ingress. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status */
  spec?: GatewaySpec
  status?: GatewayStatus
}

export interface GatewaySpec {
  /** gatewayClassName is the name of the GatewayClass that this Gateway is using. This field is immutable. */
  gatewayClassName?: string
  addresses?: GatewayAddress[]
  listeners?: GatewayListener[]
}

export interface GatewayAddress {
  type?: string
  value: string
}

export interface GatewayListener {
  name: string
  hostname?: string
  port: number
  protocol: string
  tls?: {
    mode?: string
    certificateRefs?: LocalObjectReference[]
  }
  allowedRoutes?: {
    namespaces?: {
      from?: string
      selector?: {
        matchLabels?: Record<string, string>
      }
    }
    kinds?: {
      group?: string
      kind: string
    }[]
  }
}

export interface LocalObjectReference {
  group?: string
  kind?: string
  name: string
  namespace?: string
}

export interface GatewayCondition {
  type: string
  status: string
  reason?: string
  message?: string
  lastTransitionTime?: string
}

export interface GatewayStatus {
  addresses?: GatewayAddress[]
  conditions?: GatewayCondition[]
  listeners?: {
    name: string
    attachedRoutes?: number
    supportedKinds?: {
      group?: string
      kind: string
    }[]
    conditions?: GatewayCondition[]
  }[]
}

export interface GatewayClass {
  apiVersion?: 'gateway.networking.k8s.io/v1'
  kind?: 'GatewayClass'
  metadata?: ObjectMeta
  spec?: {
    controllerName?: string
    description?: string
    parametersRef?: LocalObjectReference
  }
  status?: {
    conditions?: GatewayCondition[]
  }
}

export interface HTTPRoute {
  /** APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources */
  apiVersion?: 'gateway.networking.k8s.io/v1'
  /** Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds */
  kind?: 'HTTPRoute'
  /** Standard object's metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata */
  metadata?: ObjectMeta
  /** spec is the desired state of the HTTPRoute. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status */
  spec?: HTTPRouteSpec
  status?: {
    parents?: {
      parentRef: HTTPRouteParentRef
      controllerName?: string
      conditions?: GatewayCondition[]
    }[]
  }
}

export interface HTTPRouteParentRef {
  group?: string
  kind?: string
  name: string
  namespace?: string
  sectionName?: string
  port?: number
}

export interface HTTPRouteSpec {
  /** hostnames is a list of hostnames that this HTTPRoute matches. If empty, the HTTPRoute matches all hostnames. */
  hostnames?: string[]
  /** parentRefs is a list of references to Gateways that this HTTPRoute is attached to. */
  parentRefs?: HTTPRouteParentRef[]
}
