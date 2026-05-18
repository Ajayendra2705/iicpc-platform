module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.24"

  cluster_name    = var.cluster_name
  cluster_version = var.k8s_version

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  cluster_endpoint_public_access           = true
  enable_cluster_creator_admin_permissions = true

  cluster_addons = {
    coredns            = { most_recent = true }
    kube-proxy         = { most_recent = true }
    vpc-cni            = { most_recent = true }
    aws-ebs-csi-driver = { most_recent = true }
  }

  eks_managed_node_groups = {
    for name, cfg in var.node_groups : name => {
      instance_types = cfg.instance_types
      min_size       = cfg.min_size
      max_size       = cfg.max_size
      desired_size   = cfg.desired_size
      labels         = cfg.labels
      taints = {
        for i, t in cfg.taints : "${t.key}-${i}" => t
      }
      ami_type = "AL2023_ARM_64_STANDARD" # graviton, cheaper + lower carbon

      # Pass through extra kubelet flags via bootstrap.sh user-data. The
      # contestants pool sets --cpu-manager-policy=static so Guaranteed QoS
      # contestant pods get whole-core pinning (PS: "CPU pinning").
      pre_bootstrap_user_data = cfg.kubelet_extra_args == "" ? "" : <<-EOT
        #!/bin/bash
        echo 'KUBELET_EXTRA_ARGS="${cfg.kubelet_extra_args}"' >> /etc/kubernetes/kubelet/kubelet-config.json.d/00-extra-args.env || true
        # AL2023 path: append to the dynamic kubelet config via nodeadm.
        cat <<NODECFG > /etc/eks/nodeadm/extra-kubelet.yaml
        apiVersion: node.eks.aws/v1alpha1
        kind: NodeConfig
        spec:
          kubelet:
            flags:
            - "${cfg.kubelet_extra_args}"
        NODECFG
      EOT
    }
  }
}
