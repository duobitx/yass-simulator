# hwevents — notes / gotchas

Discovered while testing on cf cluster (2026-05-28). All fixed in the
package but worth keeping for future readers.

## 1. `remount,ro,bind` does NOT propagate over `mountPropagation`

The Linux kernel propagates **new mounts** through shared peer groups
(Bidirectional → HostToContainer in K8s terms). It does **not**
propagate remount-flag changes. So a `mount -o remount,ro,bind X X`
done in the world-controller's mnt namespace flips X to RO only there;
the engine and agent containers still see X as RW.

**Fix**: for `DiskFull` we enter each target container's mnt namespace
via `nsenter -t <pid> -m -- mount -o remount,...` (see
`remountTargets` in `hwevents.go`). One nsenter per distinct mnt ns
suffices (we de-dup PIDs by `readlink /proc/<pid>/ns/mnt`).

Requires `CAP_SYS_ADMIN` on world-controller (already in place via
`privileged: true`).

## 2. `fuse-errorfs` started via `exec.Command(...).Start()` died

Symptom: 3 zombie `[fuse-errorfs] <defunct>` after `mountErrorFS`, no
mount visible. Manual `nohup /fuse-errorfs <path> &` from the same
shell works fine.

Root cause not fully isolated — appears related to how Go's `os/exec`
sets up the child (stdin/stdout to /dev/null + no setsid). When the
child opens `/dev/fuse` and calls `fs.Serve`, *something* in the
inherited fd / signal state causes early exit.

**Fix**: shell out via `sh -c 'nohup /fuse-errorfs <path> >LOG 2>&1 &'`.
The shell does the right setsid/redirect dance and the daemon stays
alive. `fusermount -u <path>` at clear time triggers `fs.Serve` to
return cleanly.

If we ever need to drop the shell hop, the path to debug is:
- inherit stderr (`cmd.Stderr = os.Stderr`) to see the log line that
  precedes the exit;
- try `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}`.

## 3. Stuck Terminating namespace when fuse-errorfs is still mounted

If a pod is deleted while a `fuse-errorfs` daemon is still mounted on
its volumes, kubelet can't unmount the emptyDir and the pod hangs
Terminating forever. Workaround in test runs:

    kubectl delete pod <name> --grace-period=0 --force

Long-term fix candidates:
- world-controller catches SIGTERM and runs `fusermount -u` on all
  hwevents mounts before exiting;
- a Pod preStop hook on world-controller that does the same.

## 4. `:latest` tag is sticky

The operator's `INTERNAL_COMPONENTS_VERSION` env-var defaults to
`latest`. When testing, point it at a unique tag (`hardware-events-…`)
so an unrelated `:latest` repush doesn't poison the test image.
`kubectl set env deploy/yass-controller-manager … INTERNAL_COMPONENTS_VERSION=<tag>`
+ delete the experiment ns to force pod recreation.

## 5. `metrics-bridge` deployment is create-if-not-exists

Operator never reconciles an existing `metrics-bridge` deployment to a
new image. To force a refresh of bridge metrics (e.g. to pick up the
new `yass_hardware_event_*` counters), delete the deployment — the
operator recreates it on the next reconcile using the new image.
