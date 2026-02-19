# Running RamenDR on OCM Without ACM

This document describes how RamenDR can operate on a plain Open Cluster Management (OCM) hub
without requiring Red Hat Advanced Cluster Management (ACM). It details the prerequisites,
architectural changes, workflow differences, and known limitations.

## Overview

RamenDR now detects at startup whether ACM is installed on the hub cluster. When ACM is present,
Ramen uses the existing ACM-based mechanisms (Policy chains, PlacementRule, ManagedClusterAddOn).
When ACM is absent, Ramen falls back to OCM-native alternatives (ManifestWork, Placement only).

This detection is fully automatic — no configuration flags or environment variables are needed.

```
Hub Startup
    |
    v
DetectACM() — checks for "policies.policy.open-cluster-management.io" CRD
    |
    +---> CRD found:  ACM backends (backward compatible, identical to previous behavior)
    |
    +---> CRD absent: OCM-native backends (ManifestWork-based, Placement only)
```

## Prerequisites

### OCM Hub Prerequisites

The following components must be installed on the hub cluster:

| Component | Required | Notes |
|-----------|----------|-------|
| OCM Registration Operator | Yes | Core OCM — manages ManagedCluster registration |
| OCM Work Agent | Yes | Core OCM — enables ManifestWork delivery to managed clusters |
| OCM Placement Controller | Yes | Core OCM — handles Placement scheduling decisions |
| OCM Foundation (ManagedClusterView) | Yes | OCM add-on — enables hub-to-spoke resource queries |
| RamenDR Hub Operator | Yes | Deployed normally via OLM or manifests |

### Managed Cluster Prerequisites

The following must be present on each managed cluster:

| Component | Required | Notes |
|-----------|----------|-------|
| OCM Klusterlet | Yes | Core OCM — agent that processes ManifestWork and ManagedClusterView |
| RamenDR DR Cluster Operator | Yes | Deployed via ManifestWork by the hub |
| VolSync Operator | Yes | Must be **pre-installed manually** (see note below) |
| CSI driver with replication support | Yes | Same as ACM — driver-specific |

**Important: VolSync is not auto-deployed on OCM hubs.** On ACM hubs, VolSync is deployed
automatically via `ManagedClusterAddOn`. On OCM hubs, VolSync must be installed on each
managed cluster before enabling DR. The hub operator logs guidance if VolSync is not detected.

### What is NOT required on OCM hubs

These ACM-specific components are not needed:

- Policy Controller (`policy.open-cluster-management.io`)
- ConfigurationPolicy Controller (`open-cluster-management.io/config-policy-controller`)
- Governance Policy Propagator (`open-cluster-management.io/governance-policy-propagator`)
- PlacementRule Controller (`apps.open-cluster-management.io`)
- ManagedClusterAddOn infrastructure (`addon.open-cluster-management.io`)

## Workflow Differences: ACM Hub vs OCM Hub

### For Application Teams

| Action | ACM Hub | OCM Hub |
|--------|---------|---------|
| Create DRPlacementControl | Reference a PlacementRule or Placement | **Placement only** — PlacementRule returns an error with migration guidance |
| S3 secret distribution | Transparent (Policy chain) | Transparent (ManifestWork) — same user experience |
| VolSync PSK secrets | Transparent (Policy chain) | Transparent (ManifestWork) — same user experience |
| Failover / Relocate | Identical workflow | Identical workflow |

### For Cluster Administrators

| Action | ACM Hub | OCM Hub |
|--------|---------|---------|
| Install VolSync | Automatic via ManagedClusterAddOn | Manual: install VolSync operator on each managed cluster |
| Register managed clusters | Standard ACM import | Standard OCM registration (klusterlet) |
| Hub operator configuration | No special config needed | No special config needed (auto-detection) |

### PlacementRule to Placement Migration

Users migrating from ACM to OCM must update DRPlacementControl resources to reference
Placement instead of PlacementRule:

**Before (ACM — PlacementRule):**
```yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: DRPlacementControl
metadata:
  name: my-app-drpc
spec:
  placementRef:
    kind: PlacementRule
    name: my-app-placement
  drPolicyRef:
    name: my-dr-policy
```

**After (OCM — Placement):**
```yaml
apiVersion: ramendr.openshift.io/v1alpha1
kind: DRPlacementControl
metadata:
  name: my-app-drpc
spec:
  placementRef:
    kind: Placement
    name: my-app-placement
  drPolicyRef:
    name: my-dr-policy
```

The corresponding Placement resource uses the OCM API:
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: my-app-placement
  annotations:
    cluster.open-cluster-management.io/experimental-scheduling-disable: "true"
spec:
  predicates:
    - requiredClusterSelector:
        labelSelector:
          matchLabels:
            region: us-east
```

## Architecture: What Changed

### New Package: `internal/controller/acm/`

A new package provides interface-based abstraction over ACM-specific operations.
Each interface has an ACM backend (preserving existing behavior) and an OCM-native backend.

| Interface | ACM Backend | OCM Backend |
|-----------|-------------|-------------|
| `SecretPropagator` | Policy / ConfigurationPolicy / PlacementRule / PlacementBinding chain | ManifestWork containing the target Secret |
| `VolSyncSecretPropagator` | Policy chain for VolSync PSK secrets | ManifestWork per target cluster |
| `AddonDeployer` | ManagedClusterAddOn for VolSync | CRD existence check + log guidance |
| `PlacementAdapter` | Watches and handles both PlacementRule and Placement | Watches and handles Placement only |

### Detection Mechanism

At startup, `acm.DetectACM()` checks whether the CRD
`policies.policy.open-cluster-management.io` exists on the hub cluster. This is a reliable
indicator of ACM because the Policy CRD is installed by the ACM Governance framework,
which is present on all ACM hubs.

### Files Created (14 files in `internal/controller/acm/`)

| File | Purpose |
|------|---------|
| `interfaces.go` | Interface definitions: SecretPropagator, VolSyncSecretPropagator, AddonDeployer, PlacementAdapter |
| `detect.go` | Runtime ACM detection via Policy CRD check |
| `acm_secret_propagator.go` | ACM backend: wraps existing `util.SecretsUtil` (Policy chain) |
| `ocm_secret_propagator.go` | OCM backend: ManifestWork-based secret propagation |
| `acm_volsync_propagator.go` | ACM backend: wraps existing `volsync.PropagateSecretToClusters` |
| `ocm_volsync_propagator.go` | OCM backend: ManifestWork-based VolSync PSK distribution |
| `acm_addon_deployer.go` | ACM backend: wraps `volsync.DeployVolSyncToCluster` (ManagedClusterAddOn) |
| `ocm_addon_deployer.go` | OCM backend: checks CRD existence, logs pre-install guidance |
| `acm_placement_adapter.go` | ACM backend: handles PlacementRule + Placement |
| `ocm_placement_adapter.go` | OCM backend: handles Placement only, rejects PlacementRule with guidance |
| `suite_test.go` | Ginkgo test suite runner |
| `detect_test.go` | Unit tests for ACM detection (3 tests) |
| `ocm_secret_propagator_test.go` | Unit tests for ManifestWork-based propagation (4 tests) |
| `integration_test.go` | Integration tests for dual-backend selection (6 tests) |

### Files Modified (9 existing files)

| File | Change |
|------|--------|
| `cmd/main.go` | ACM detection at startup; conditional scheme registration; backend injection into reconcilers |
| `drpolicy_controller.go` | Added `SecretPropagator` field; removed inline `SecretsUtil` creation |
| `drpolicy.go` | Functions accept `acm.SecretPropagator` instead of `*util.SecretsUtil` |
| `drcluster_controller.go` | Added `AddonDeployer` and `PlacementAdapter` fields |
| `drclusters.go` | Uses `AddonDeployer.DeployAddon()` instead of direct `volsync.DeployVolSyncToCluster()` |
| `drplacementcontrol_controller.go` | Added `PlacementAdapter` and `VolSyncSecretProp` fields; placement retrieval via adapter |
| `drplacementcontrol_watcher.go` | Placement watches delegated to adapter (PlacementRule watch conditional) |
| `drplacementcontrolvolsync.go` | VolSync secret propagation via interface; OCM guidance logging |
| `drcluster_mmode.go` | Placement retrieval via adapter for maintenance mode operations |

### Backward Compatibility

All changes are backward compatible. On ACM hubs:

- ACM detection returns `true`
- ACM-specific schemes (`plrv1`, `cpcv1`, `gppv1`) are registered
- ACM backends are used — behavior is identical to before the refactoring
- Both PlacementRule and Placement continue to work
- VolSync is still auto-deployed via ManagedClusterAddOn

No existing ACM workflows are affected.

## Known Limitations

1. **VolSync must be pre-installed on OCM hubs.** There is no automatic VolSync deployment.
   If VolSync is missing, the VRG controller on managed clusters will fail to create
   `ReplicationSource`/`ReplicationDestination` resources and will retry indefinitely.

2. **PlacementRule is not supported on OCM hubs.** Users must use the Placement API
   (`cluster.open-cluster-management.io/v1beta1`). Attempting to reference a PlacementRule
   returns a clear error with migration instructions.

3. **ManagedClusterView is required.** The OCM Foundation add-on must be installed to provide
   ManagedClusterView, which Ramen uses to query resource status on managed clusters. This is
   part of the OCM ecosystem but may not be included in a minimal OCM installation.

4. **RBAC includes unused ACM permissions.** The generated RBAC roles include permissions for
   `managedclusteraddons` which are unused on OCM hubs. These are harmless but not cleaned up
   conditionally.
