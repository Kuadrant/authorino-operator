# User guide: Preventing namespace to cluster privilege escalation (Authorino CRs)

Three fields on `Authorino` resources reach beyond the namespace they live in: `spec.clusterWide`, `spec.image` and `spec.supersedingHostSubsets`. The operator applies these fields without checking whether the person setting them should really have that much access, so anyone allowed to create `Authorino` resources in a single namespace can quietly gain cluster-wide access. `spec.supersedingHostSubsets: true` lets AuthConfigs reconciled by that instance take over strict subsets of hosts already claimed elsewhere, which is a way to hijack traffic from another tenant's AuthConfigs.

The [ValidatingAdmissionPolicy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/) below closes that gap. It blocks those fields unless the user has been given a special permission for them, and you hand that permission only to the subjects that need it to do their job.

## Prerequisites

**Kubernetes 1.30 or newer.** ValidatingAdmissionPolicy reached GA in Kubernetes 1.30, which is where the `admissionregistration.k8s.io/v1` API used throughout this guide is served. On older clusters the manifests in step 3 will be rejected.

The policy:

<table>
  <thead>
    <tr>
      <th>Policy</th>
      <th>Resource</th>
      <th>Denies</th>
      <th>ClusterRole required to allow</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td rowspan="3"><code>authorino-restrict-spec-fields</code></td>
      <td rowspan="3"><code>authorinos</code></td>
      <td><code>spec.image</code> (any non-empty value)</td>
      <td><code>set-image</code> on <code>authorinos</code></td>
    </tr>
    <tr>
      <td><code>spec.clusterWide: true</code></td>
      <td><code>set-cluster-wide</code> on <code>authorinos</code></td>
    </tr>
    <tr>
      <td><code>spec.supersedingHostSubsets: true</code></td>
      <td><code>set-superseding-host-subsets</code> on <code>authorinos</code></td>
    </tr>
  </tbody>
</table>

Follow the steps below: create the Roles that grant those permissions, bind the roles to specific SAs and Users, then apply the policy.

## 1. Create the Roles

```sh
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: authorino-set-cluster-wide
rules:
  - apiGroups: ["operator.authorino.kuadrant.io"]
    resources: ["authorinos"]
    verbs: ["set-cluster-wide"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: authorino-set-image
rules:
  - apiGroups: ["operator.authorino.kuadrant.io"]
    resources: ["authorinos"]
    verbs: ["set-image"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: authorino-set-superseding-host-subsets
rules:
  - apiGroups: ["operator.authorino.kuadrant.io"]
    resources: ["authorinos"]
    verbs: ["set-superseding-host-subsets"]
EOF
```

## 2. Grant the access to the restricted fields

> [!IMPORTANT]
> Before granting anyone else, grant the **controllers and GitOps agents that write `Authorino` CRs** the permissions matching the restricted fields their manifests set, otherwise they cannot manage `Authorino` CRs once the policy is active. This includes any CI or GitOps ServiceAccount reconciling a manifest that pins a restricted field: a pipeline that sets `spec.image` is blocked unless it holds `set-image`, just like a user would be. Grant only the permissions actually needed, not all three.

On a Kuadrant installation the writer is the **kuadrant-operator** ServiceAccount (the standard deployment uses `kuadrant-operator-controller-manager` in `kuadrant-system`), which creates the `Authorino` CR with `spec.clusterWide: true`. It never sets `spec.image`, so `set-cluster-wide` is the only permission it always needs. Replace `<writer-sa>` and `<writer-namespace>` with the ServiceAccount writing the `Authorino` CRs:

```sh
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: authorino-set-cluster-wide
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authorino-set-cluster-wide
subjects:
  - kind: ServiceAccount
    name: <writer-sa>
    namespace: <writer-namespace>
EOF
```

Older Kuadrant versions also set `spec.supersedingHostSubsets: true` on the `Authorino` CR. To check whether you also need this role, try this command:

```sh
kubectl get authorinos -A -o json | jq -r '.items[]
  | select(.spec.supersedingHostSubsets // false)
  | "\(.metadata.namespace)/\(.metadata.name)"'
```

If that prints anything, also bind:

```sh
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: authorino-set-superseding-host-subsets
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authorino-set-superseding-host-subsets
subjects:
  - kind: ServiceAccount
    name: <writer-sa>
    namespace: <writer-namespace>
EOF
```

Then grant access to your own ServiceAccounts and Users. Use the RoleBindings below as a template. Replace the placeholders (`<sa-name>`, `<namespace-of-sa>`, `<authorino-namespace>`) with the appropriate values, and keep only the bindings for the fields that subject actually needs to set.


```sh
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rb-set-cluster-wide
  namespace: <authorino-namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authorino-set-cluster-wide
subjects:
  - kind: ServiceAccount
    name: <sa-name>
    namespace: <namespace-of-sa>
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rb-set-image
  namespace: <authorino-namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authorino-set-image
subjects:
  - kind: ServiceAccount
    name: <sa-name>
    namespace: <namespace-of-sa>
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rb-set-superseding-host-subsets
  namespace: <authorino-namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authorino-set-superseding-host-subsets
subjects:
  - kind: ServiceAccount
    name: <sa-name>
    namespace: <namespace-of-sa>
EOF
```

## 3. Create the ValidatingAdmissionPolicy

First list the existing `Authorino` CRs that already enable a restricted field. Every resource printed here becomes unwritable, even for unrelated changes, for any subject that does not hold the matching permission, so make sure whoever manages them was covered in step 2:

```sh
kubectl get authorinos -A -o json | jq -r '.items[]
  | select((.spec.clusterWide // false) or (.spec.supersedingHostSubsets // false) or ((.spec.image // "") != ""))
  | "\(.metadata.namespace)/\(.metadata.name)"'
```

Then apply the policy:

```sh
kubectl apply -f - <<'EOF'
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: authorino-restrict-spec-fields
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
      - apiGroups: ["operator.authorino.kuadrant.io"]
        apiVersions: ["v1beta1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["authorinos"]
  variables:
    - name: isExemptClusterWide
      expression: "authorizer.requestResource.check('set-cluster-wide').allowed()"
    - name: isExemptImage
      expression: "authorizer.requestResource.check('set-image').allowed()"
    - name: isExemptSupersedingHostSubsets
      expression: "authorizer.requestResource.check('set-superseding-host-subsets').allowed()"
    - name: wantsClusterWide
      expression: "has(object.spec.clusterWide) && object.spec.clusterWide"
    - name: wantsImage
      expression: "has(object.spec.image) && object.spec.image != ''"
    - name: wantsSupersedingHostSubsets
      expression: "has(object.spec.supersedingHostSubsets) && object.spec.supersedingHostSubsets"
  validations:
    - expression: "!variables.wantsImage || variables.isExemptImage"
      message: "spec.image can only be set by a subject granted the 'set-image' permission on authorinos"
      reason: Forbidden
    - expression: "!variables.wantsClusterWide || variables.isExemptClusterWide"
      message: "spec.clusterWide: true can only be set by a subject granted the 'set-cluster-wide' permission on authorinos"
      reason: Forbidden
    - expression: "!variables.wantsSupersedingHostSubsets || variables.isExemptSupersedingHostSubsets"
      message: "spec.supersedingHostSubsets: true can only be set by a subject granted the 'set-superseding-host-subsets' permission on authorinos"
      reason: Forbidden
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: authorino-restrict-spec-fields-binding
spec:
  policyName: authorino-restrict-spec-fields
  validationActions: ["Deny"]
EOF
```

> [!WARNING]
> Enforcement is strict, there is no grandfathering. On **every** create and update, any resource that enables a restricted field (a non-empty `spec.image`, `spec.clusterWide: true` or `spec.supersedingHostSubsets: true`) is **rejected** unless the requesting subject holds the matching permission. This includes updates to resources that already exist: once the policy is active, a subject without the permission cannot update such a resource, or even change unrelated fields, until it either **drops the restricted field** (removes it, or sets `spec.image` to an empty string and `spec.clusterWide` / `spec.supersedingHostSubsets` to `false`) or is **granted the corresponding ClusterRole** (steps 1–2). To avoid breaking existing workloads, grant the required Roles and RoleBindings **before** applying the policy.

## 4. Verifying the VAP

### A normal user is blocked

Try to create resources that break the rules. Run these as a regular user (one *without* the permissions) and all of them should be **rejected**:

> [!NOTE]
> Don't run these as a cluster admin. Anything with wildcard access (`verbs: ["*"]`) — which cluster admins have — satisfies the `set-cluster-wide` / `set-image` / `set-superseding-host-subsets` checks and is treated as exempt, so the request would be **allowed** and a real cluster-wide instance created. Use an ordinary user (or `--as=<unauthorized-subject>`) to see the policy block.

```sh
# Authorino with clusterWide: true — should be DENIED
kubectl apply --as=<unauthorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-cluster-wide-1
  namespace: <namespace>
spec:
  clusterWide: true
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

```sh
# Authorino with a custom spec.image — should be DENIED
kubectl apply --as=<unauthorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-image-1
  namespace: <namespace>
spec:
  image: example.com/authorino:custom
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

```sh
# Authorino with supersedingHostSubsets: true — should be DENIED
kubectl apply --as=<unauthorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-superseding-host-subsets-1
  namespace: <namespace>
spec:
  supersedingHostSubsets: true
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

You should get an error like this instead of the resource being created:

```text
... is forbidden: ValidatingAdmissionPolicy 'authorino-restrict-spec-fields' ... denied request: spec.clusterWide: true can only be set by a subject granted the 'set-cluster-wide' permission on authorinos
```

### An authorized subject is allowed

Now run the same requests as a subject that holds the matching permission (granted in steps 1–2). All of them should be **admitted**. Replace `<authorized-subject>` with the subject you granted the permission to (e.g. `system:serviceaccount:<namespace>:<sa>`):

```sh
# Authorino with clusterWide: true, as a subject granted 'set-cluster-wide' — should be ALLOWED
kubectl apply --as=<authorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-cluster-wide-2
  namespace: <namespace>
spec:
  clusterWide: true
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

```sh
# Authorino with a custom spec.image, as a subject granted 'set-image' — should be ALLOWED
kubectl apply --as=<authorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-image-2
  namespace: <namespace>
spec:
  image: example.com/authorino:custom
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

```sh
# Authorino with supersedingHostSubsets: true, as a subject granted 'set-superseding-host-subsets' — should be ALLOWED
kubectl apply --as=<authorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-superseding-host-subsets-2
  namespace: <namespace>
spec:
  supersedingHostSubsets: true
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

### Resources without the restricted fields are always allowed

The policy only looks at the restricted fields. A resource that leaves them unset (or `false`) is admitted for **any** subject, whether or not it holds a permission:

```sh
# Namespaced Authorino (clusterWide and supersedingHostSubsets omitted, no custom image) — should be ALLOWED even for an unauthorized subject
kubectl apply --as=<unauthorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-namespaced-1
  namespace: <namespace>
spec:
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

### Updates are re-checked, not just creates

Because the policy matches `UPDATE` as well as `CREATE`, it re-evaluates on every change. A subject without the permission can still edit **unrelated** fields of a compliant resource, but is blocked the moment it tries to switch a restricted field on. Using the namespaced instance created above:

```sh
# Change an unrelated field (logLevel) on the namespaced Authorino — should be ALLOWED
kubectl apply --as=<unauthorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-namespaced-1
  namespace: <namespace>
spec:
  logLevel: debug
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

```sh
# Flip the same instance to clusterWide: true — should be DENIED
kubectl apply --as=<unauthorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-namespaced-1
  namespace: <namespace>
spec:
  clusterWide: true
  logLevel: debug
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

### Permissions bound with a RoleBinding are namespace-scoped

The exemption check runs against the namespace of the resource being admitted. If you grant the permission with a `RoleBinding` (rather than a `ClusterRoleBinding`), the subject is exempt only in that namespace.

```sh
# Subject granted 'set-cluster-wide' via a RoleBinding in <namespace-a> — should be ALLOWED
kubectl apply --as=<authorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-cluster-wide-3
  namespace: <namespace-a>
spec:
  clusterWide: true
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```

```sh
# Same subject, same request, in <namespace-b> where it has no binding — should be DENIED
kubectl apply --as=<authorized-subject> -f - <<'EOF'
apiVersion: operator.authorino.kuadrant.io/v1beta1
kind: Authorino
metadata:
  name: authorino-cluster-wide-3
  namespace: <namespace-b>
spec:
  clusterWide: true
  listener:
    tls:
      enabled: false
  oidcServer:
    tls:
      enabled: false
EOF
```
