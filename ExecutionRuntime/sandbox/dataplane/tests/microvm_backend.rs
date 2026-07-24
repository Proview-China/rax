mod common;

use std::collections::BTreeMap;
use std::env;
use std::fs;
use std::io::Write as _;
use std::os::unix::fs::PermissionsExt as _;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};

use praxis_sandbox_dataplane::contract::{
    DispatchRequestV1, EnforcementPhaseV1, MicroVmPayloadV1, ProviderInspectionTargetV1,
    ProviderPayloadV1,
};
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::microvm::{MicroVmArtifactBinding, MicroVmConfig, MicroVmProvider};
use praxis_sandbox_dataplane::provider::{Provider, provider_result};
use sha2::{Digest as _, Sha256};

#[tokio::test]
async fn pinned_artifacts_allocate_without_starting_a_vm() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let kernel = temporary.path().join("kernel");
    let initramfs = temporary.path().join("initramfs");
    fs::write(&kernel, b"kernel-fixture").unwrap_or_else(|error| panic!("kernel: {error}"));
    fs::write(&initramfs, b"initramfs-fixture")
        .unwrap_or_else(|error| panic!("initramfs: {error}"));
    let fixture = Fixture::new(temporary.path(), &kernel, &initramfs);
    let provider = fixture.provider();
    provider
        .probe()
        .await
        .unwrap_or_else(|error| panic!("probe: {error}"));
    let prepare = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-1",
        EnforcementPhaseV1::Prepare,
    );
    provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    let execute = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-1",
        EnforcementPhaseV1::Execute,
    );
    let allocated = provider
        .execute_prepared(&execute)
        .await
        .unwrap_or_else(|error| panic!("allocate: {error}"));
    assert_eq!(allocated.observation.state, "microvm_allocated");
}

#[tokio::test]
async fn artifact_drift_and_kvm_downgrade_fail_closed() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let kernel = temporary.path().join("kernel");
    let initramfs = temporary.path().join("initramfs");
    fs::write(&kernel, b"kernel-fixture").unwrap_or_else(|error| panic!("kernel: {error}"));
    fs::write(&initramfs, b"initramfs-fixture")
        .unwrap_or_else(|error| panic!("initramfs: {error}"));
    let fixture = Fixture::new(temporary.path(), &kernel, &initramfs);
    let mut wrong = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-wrong",
        EnforcementPhaseV1::Prepare,
    );
    let ProviderPayloadV1::QemuMicrovm(mut payload) = wrong.payload.clone() else {
        panic!("microVM fixture payload")
    };
    payload.kernel_digest = common::digest("wrong-kernel");
    wrong.payload = ProviderPayloadV1::QemuMicrovm(payload);
    wrong = wrong
        .seal()
        .unwrap_or_else(|error| panic!("reseal: {error}"));
    let error = common::must_error(fixture.provider().prepare(&wrong).await);
    assert_eq!(error.reason, ClosedReason::InvalidDigest);

    let mut downgraded = fixture.config();
    downgraded.require_kvm = false;
    let error = common::must_error(MicroVmProvider::new(downgraded).probe().await);
    assert_eq!(error.reason, ClosedReason::InvalidContract);
}

#[tokio::test]
async fn concurrent_different_allocations_have_one_stable_artifact_winner() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let kernel = temporary.path().join("kernel");
    let alternate_kernel = temporary.path().join("kernel-alternate");
    let initramfs = temporary.path().join("initramfs");
    fs::write(&kernel, b"kernel-fixture").unwrap_or_else(|error| panic!("kernel: {error}"));
    fs::write(&alternate_kernel, b"kernel-alternate")
        .unwrap_or_else(|error| panic!("alternate kernel: {error}"));
    fs::write(&initramfs, b"initramfs-fixture")
        .unwrap_or_else(|error| panic!("initramfs: {error}"));
    let fixture = Fixture::new(temporary.path(), &kernel, &initramfs);
    let mut config = fixture.config();
    let alternate_digest = file_digest(&alternate_kernel);
    config.kernel_bindings.insert(
        "kernel-2".to_owned(),
        MicroVmArtifactBinding {
            path: alternate_kernel,
            digest: alternate_digest.clone(),
        },
    );
    let first = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-race-a",
        EnforcementPhaseV1::Execute,
    );
    let mut second = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-race-b",
        EnforcementPhaseV1::Execute,
    );
    let ProviderPayloadV1::QemuMicrovm(mut payload) = second.payload.clone() else {
        panic!("microVM payload")
    };
    payload.kernel_binding_id = "kernel-2".to_owned();
    payload.kernel_digest = alternate_digest;
    second.payload = ProviderPayloadV1::QemuMicrovm(payload);
    second = second
        .seal()
        .unwrap_or_else(|error| panic!("alternate request: {error}"));

    let mut tasks = Vec::new();
    for index in 0..64 {
        let provider = MicroVmProvider::new(config.clone());
        let request = if index % 2 == 0 {
            first.clone()
        } else {
            second.clone()
        };
        tasks.push(tokio::spawn(async move {
            provider.execute_prepared(&request).await
        }));
    }
    let mut first_won = false;
    let mut second_won = false;
    for (index, task) in tasks.into_iter().enumerate() {
        match task.await.unwrap_or_else(|error| panic!("join: {error}")) {
            Ok(_) if index % 2 == 0 => first_won = true,
            Ok(_) => second_won = true,
            Err(error) => assert_eq!(error.reason, ClosedReason::Conflict),
        }
    }
    assert_ne!(first_won, second_won, "two artifact branches both won");
}

#[tokio::test]
#[ignore = "requires PRAXIS_TEST_KERNEL and local KVM"]
async fn live_kvm_boots_an_independent_kernel_and_cleans_up() {
    let kernel = PathBuf::from(
        env::var_os("PRAXIS_TEST_KERNEL")
            .unwrap_or_else(|| panic!("PRAXIS_TEST_KERNEL is required")),
    );
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let initramfs = build_initramfs(temporary.path());
    let fixture = Fixture::new(temporary.path(), &kernel, &initramfs);
    let provider = fixture.provider();
    provider
        .probe()
        .await
        .unwrap_or_else(|error| panic!("probe: {error}"));
    provider
        .execute_prepared(&fixture.request(
            "praxis.sandbox/allocate",
            "allocate-live",
            EnforcementPhaseV1::Execute,
        ))
        .await
        .unwrap_or_else(|error| panic!("allocate: {error}"));
    let boot = provider
        .execute_prepared(&fixture.request(
            "praxis.sandbox/activate",
            "activate-live",
            EnforcementPhaseV1::Execute,
        ))
        .await
        .unwrap_or_else(|error| panic!("boot: {error}"));
    assert_eq!(boot.observation.state, "exited:0");
    let serial = find_serial(temporary.path()).unwrap_or_else(|| panic!("serial log absent"));
    let output = fs::read_to_string(serial).unwrap_or_else(|error| panic!("serial: {error}"));
    assert!(
        output.contains("PRAXIS_MICROVM_OK"),
        "serial output: {output}"
    );
    let cleanup = provider
        .cleanup(&fixture.request(
            "praxis.sandbox/cleanup",
            "cleanup-live",
            EnforcementPhaseV1::Execute,
        ))
        .await
        .unwrap_or_else(|error| panic!("cleanup: {error}"));
    assert_eq!(cleanup.observation.state, "cleanup_absent");
}

#[tokio::test]
#[ignore = "requires PRAXIS_TEST_KERNEL and local KVM"]
async fn live_kvm_inspect_fence_release_and_residual_are_independent() {
    let kernel = PathBuf::from(
        env::var_os("PRAXIS_TEST_KERNEL")
            .unwrap_or_else(|| panic!("PRAXIS_TEST_KERNEL is required")),
    );
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let initramfs = build_initramfs_with_body(
        temporary.path(),
        "echo PRAXIS_MICROVM_RUNNING\n/bin/busybox sleep 30\n/bin/busybox poweroff -f\n",
    );
    let fixture = Fixture::new(temporary.path(), &kernel, &initramfs);
    let provider = fixture.provider();
    provider
        .execute_prepared(&fixture.request(
            "praxis.sandbox/allocate",
            "allocate-fence",
            EnforcementPhaseV1::Execute,
        ))
        .await
        .unwrap_or_else(|error| panic!("allocate: {error}"));
    let activate = fixture.request(
        "praxis.sandbox/activate",
        "activate-fence",
        EnforcementPhaseV1::Execute,
    );
    let expected_attempt = provider_result(&activate, "running")
        .unwrap_or_else(|error| panic!("attempt: {error}"))
        .attempt;
    let inspect = fixture.inspect_request(&activate, expected_attempt);
    let boot_provider = fixture.provider();
    let boot = tokio::spawn(async move { boot_provider.execute_prepared(&activate).await });

    let mut running = false;
    for _ in 0..100 {
        match provider.inspect(&inspect).await {
            Ok(result) if result.observation.state == "running" => {
                running = true;
                break;
            }
            Ok(_) | Err(_) => tokio::time::sleep(std::time::Duration::from_millis(20)).await,
        }
    }
    assert!(running, "microVM never reached inspectable running state");

    let release_error = common::must_error(
        provider
            .release(&fixture.request(
                "praxis.sandbox/release",
                "release-live",
                EnforcementPhaseV1::Execute,
            ))
            .await,
    );
    assert_eq!(release_error.reason, ClosedReason::Conflict);
    let fenced = provider
        .fence(&fixture.request(
            "praxis.sandbox/fence",
            "fence-live",
            EnforcementPhaseV1::Execute,
        ))
        .await
        .unwrap_or_else(|error| panic!("fence: {error}"));
    assert_eq!(fenced.observation.state, "fenced");
    let completed = boot
        .await
        .unwrap_or_else(|error| panic!("join: {error}"))
        .unwrap_or_else(|error| panic!("boot completion: {error}"));
    assert!(
        completed.observation.state.starts_with("exited:"),
        "unexpected completion: {}",
        completed.observation.state
    );
    let cleanup = provider
        .cleanup(&fixture.request(
            "praxis.sandbox/cleanup",
            "cleanup-fenced",
            EnforcementPhaseV1::Execute,
        ))
        .await
        .unwrap_or_else(|error| panic!("cleanup: {error}"));
    assert_eq!(cleanup.observation.state, "cleanup_absent");
}

struct Fixture {
    state: PathBuf,
    kernel: PathBuf,
    initramfs: PathBuf,
    kernel_digest: String,
    initramfs_digest: String,
    qemu_digest: String,
}

impl Fixture {
    fn new(root: &Path, kernel: &Path, initramfs: &Path) -> Self {
        Self {
            state: root.join("state"),
            kernel: kernel.to_path_buf(),
            initramfs: initramfs.to_path_buf(),
            kernel_digest: file_digest(kernel),
            initramfs_digest: file_digest(initramfs),
            qemu_digest: file_digest(Path::new("/usr/bin/qemu-system-x86_64")),
        }
    }

    fn config(&self) -> MicroVmConfig {
        MicroVmConfig {
            qemu_path: PathBuf::from("/usr/bin/qemu-system-x86_64"),
            qemu_digest: self.qemu_digest.clone(),
            state_directory: self.state.clone(),
            kernel_cmdline: "console=ttyS0 panic=-1 rdinit=/init".to_owned(),
            require_kvm: true,
            kernel_bindings: BTreeMap::from([(
                "kernel-1".to_owned(),
                MicroVmArtifactBinding {
                    path: self.kernel.clone(),
                    digest: self.kernel_digest.clone(),
                },
            )]),
            initramfs_bindings: BTreeMap::from([(
                "initramfs-1".to_owned(),
                MicroVmArtifactBinding {
                    path: self.initramfs.clone(),
                    digest: self.initramfs_digest.clone(),
                },
            )]),
        }
    }

    fn provider(&self) -> MicroVmProvider {
        MicroVmProvider::new(self.config())
    }

    fn payload(&self) -> ProviderPayloadV1 {
        ProviderPayloadV1::QemuMicrovm(MicroVmPayloadV1 {
            kernel_binding_id: "kernel-1".to_owned(),
            kernel_digest: self.kernel_digest.clone(),
            initramfs_binding_id: "initramfs-1".to_owned(),
            initramfs_digest: self.initramfs_digest.clone(),
            vcpus: 1,
            memory_mib: 128,
            network_deny_all: true,
            wall_clock_timeout_millis: 30_000,
            inspection_target: None,
        })
    }

    fn request(
        &self,
        effect_kind: &str,
        attempt_id: &str,
        phase: EnforcementPhaseV1,
    ) -> DispatchRequestV1 {
        let mut request = common::request_with_payload(phase, self.payload());
        request.request_id = format!("{attempt_id}-{phase:?}");
        request.effect_kind = effect_kind.to_owned();
        request.effect_id = format!("effect-{attempt_id}");
        request.attempt_id = attempt_id.to_owned();
        request.sandbox_attempt.id = attempt_id.to_owned();
        request.sandbox_attempt.digest = common::digest(attempt_id);
        request
            .runtime_enforcement
            .effect_id
            .clone_from(&request.effect_id);
        request.runtime_enforcement.attempt_id = attempt_id.to_owned();
        request.runtime_enforcement.receipt_digest = common::digest(&request.request_id);
        request
            .seal()
            .unwrap_or_else(|error| panic!("microVM request: {error}"))
    }

    fn inspect_request(
        &self,
        original: &DispatchRequestV1,
        provider_attempt: praxis_sandbox_dataplane::contract::ExactRefV1,
    ) -> DispatchRequestV1 {
        let ProviderPayloadV1::QemuMicrovm(mut payload) = original.payload.clone() else {
            panic!("microVM original payload")
        };
        payload.inspection_target = Some(ProviderInspectionTargetV1 {
            original_effect_kind: original.effect_kind.clone(),
            original_attempt_id: original.attempt_id.clone(),
            provider_attempt,
            original_request_digest: original.digest.clone(),
            original_payload_digest: original.payload_digest.clone(),
        });
        let mut request = self.request(
            "praxis.sandbox/open",
            "inspect-live",
            EnforcementPhaseV1::Execute,
        );
        request.effect_kind = "praxis.sandbox/inspect".to_owned();
        request.payload = ProviderPayloadV1::QemuMicrovm(payload);
        request
            .seal()
            .unwrap_or_else(|error| panic!("microVM inspect: {error}"))
    }
}

fn build_initramfs(root: &Path) -> PathBuf {
    build_initramfs_with_body(root, "echo PRAXIS_MICROVM_OK\n/bin/busybox poweroff -f\n")
}

fn build_initramfs_with_body(root: &Path, body: &str) -> PathBuf {
    let tree = root.join("initramfs-tree");
    fs::create_dir_all(tree.join("bin")).unwrap_or_else(|error| panic!("initramfs dirs: {error}"));
    fs::copy("/usr/bin/busybox", tree.join("bin/busybox"))
        .unwrap_or_else(|error| panic!("busybox: {error}"));
    let init = tree.join("init");
    fs::write(&init, format!("#!/bin/busybox sh\n{body}"))
        .unwrap_or_else(|error| panic!("init: {error}"));
    fs::set_permissions(&init, fs::Permissions::from_mode(0o755))
        .unwrap_or_else(|error| panic!("chmod init: {error}"));
    let archive = root.join("initramfs.cpio");
    let output = fs::File::create(&archive).unwrap_or_else(|error| panic!("archive: {error}"));
    let mut child = Command::new("/usr/bin/cpio")
        .args(["-o", "-H", "newc", "--quiet"])
        .current_dir(&tree)
        .stdin(Stdio::piped())
        .stdout(Stdio::from(output))
        .stderr(Stdio::null())
        .spawn()
        .unwrap_or_else(|error| panic!("cpio: {error}"));
    child
        .stdin
        .as_mut()
        .unwrap_or_else(|| panic!("cpio stdin"))
        .write_all(b".\nbin\nbin/busybox\ninit\n")
        .unwrap_or_else(|error| panic!("cpio list: {error}"));
    let status = child
        .wait()
        .unwrap_or_else(|error| panic!("cpio wait: {error}"));
    assert!(status.success(), "cpio failed: {status}");
    archive
}

fn find_serial(root: &Path) -> Option<PathBuf> {
    let mut stack = vec![root.to_path_buf()];
    while let Some(path) = stack.pop() {
        for entry in fs::read_dir(path).ok()? {
            let entry = entry.ok()?;
            if entry.file_name() == "serial.log" {
                return Some(entry.path());
            }
            if entry.file_type().ok()?.is_dir() {
                stack.push(entry.path());
            }
        }
    }
    None
}

fn file_digest(path: &Path) -> String {
    let bytes = fs::read(path).unwrap_or_else(|error| panic!("read artifact: {error}"));
    format!("sha256:{}", hex::encode(Sha256::digest(bytes)))
}
