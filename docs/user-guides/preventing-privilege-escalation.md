# User guide: Preventing namespace to cluster privilege escalation (Authorino CRs)

Two fields on `Authorino` resources reach beyond the namespace they live in: `spec.clusterWide` and `spec.image`. The operator applies these fields without checking whether the person setting them should really have that much access, so anyone allowed to create `Authorino` resources in a single namespace can quietly gain cluster-wide access.

The [ValidatingAdmissionPolicy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/) below closes that gap. It blocks those fields unless the user has been given a special permission for them, and you hand that permission only to the subjects that need it to do their job.

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
      <td rowspan="2"><code>authorino-restrict-spec-fields</code></td>
      <td rowspan="2"><code>authorinos</code></td>
      <td><code>spec.image</code> (any non-empty value)</td>
      <td><code>set-image</code> on <code>authorinos</code></td>
    </tr>
    <tr>
      <td><code>spec.clusterWide: true</code></td>
      <td><code>set-cluster-wide</code> on <code>authorinos</code></td>
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
EOF
```

## 2. Grant the access to the restricted fields

> [!IMPORTANT]
> Before granting anyone else, grant the **operator's own ServiceAccount** the `set-cluster-wide` and `set-image` permissions, otherwise it cannot manage `Authorino` CRs once the policy is active. In order to bind both ClusterRoles to the operator's ServiceAccount, replace `<operator-sa>` with the operator's ServiceAccount name (the standard deployment uses `authorino-operator`) and `<operator-namespace>` with the namespace where the operator is running:

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
    name: <operator-sa>
    namespace: <operator-namespace>
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: authorino-set-image
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authorino-set-image
subjects:
  - kind: ServiceAccount
    name: <operator-sa>
    namespace: <operator-namespace>
EOF
```

Then grant access to your own ServiceAccounts and Users. Use the RoleBindings below as a template. Replace the placeholders (`<sa-name>`, `<namespace-of-sa>`, `<authorino-namespace>`) with the appropriate values.

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
EOF
```

## 3. Create the ValidatingAdmissionPolicy

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
    - name: wantsClusterWide
      expression: "has(object.spec.clusterWide) && object.spec.clusterWide"
    - name: wantsImage
      expression: "has(object.spec.image) && object.spec.image != ''"
  validations:
    - expression: "!variables.wantsImage || variables.isExemptImage"
      message: "spec.image can only be set by a subject granted the 'set-image' permission on authorinos"
      reason: Forbidden
    - expression: "!variables.wantsClusterWide || variables.isExemptClusterWide"
      message: "spec.clusterWide: true can only be set by a subject granted the 'set-cluster-wide' permission on authorinos"
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
> Enforcement is strict, there is no grandfathering. On **every** create and update, any resource that enables a restricted field (a non-empty `spec.image` or `spec.clusterWide: true`) is **rejected** unless the requesting subject holds the matching permission. This includes updates to resources that already exist: once the policy is active, a subject without the permission cannot update such a resource, or even change unrelated fields, until it either **drops the restricted field** (removes it, or sets `spec.image` to an empty string and `spec.clusterWide` to `false`) or is **granted the corresponding ClusterRole** (steps 1–2). To avoid breaking existing workloads, grant the required Roles and RoleBindings **before** applying the policy.

## 4. Verifying the VAP

### A normal user is blocked

Try to create resources that break the rules. Run these as a regular user (one *without* the permissions) and both should be **rejected**:

> [!NOTE]
> Don't run these as a cluster admin. Anything with wildcard access (`verbs: ["*"]`) — which cluster admins have — satisfies the `set-cluster-wide` / `set-image` checks and is treated as exempt, so the request would be **allowed** and a real cluster-wide instance created. Use an ordinary user (or `--as=<unauthorized-subject>`) to see the policy block.

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

You should get an error like this instead of the resource being created:

```text
... is forbidden: ValidatingAdmissionPolicy 'authorino-restrict-spec-fields' ... denied request: spec.clusterWide: true can only be set by a subject granted the 'set-cluster-wide' permission on authorinos
```

### An authorized subject is allowed

Now run the same requests as a subject that holds the matching permission (granted in steps 1–2). Both should be **admitted**. Replace `<authorized-subject>` with the subject you granted the permission to (e.g. `system:serviceaccount:<namespace>:<sa>`):

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

### Resources without the restricted fields are always allowed

The policy only looks at the restricted fields. A resource that leaves them unset (or `false`) is admitted for **any** subject, whether or not it holds a permission:

```sh
# Namespaced Authorino (clusterWide omitted, no custom image) — should be ALLOWED even for an unauthorized subject
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
