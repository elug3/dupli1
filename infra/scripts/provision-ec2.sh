#!/usr/bin/env bash
# Provision a single EC2 instance for Dupli1 and attach an Elastic IP.
#
# Usage:
#   bash infra/scripts/provision-ec2.sh
#
# Outputs instance ID, public IP, and SSH command.
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
INSTANCE_TYPE="${INSTANCE_TYPE:-t3.large}"
INSTANCE_NAME="${INSTANCE_NAME:-dupli1-app}"
KEY_NAME="${KEY_NAME:-dupli1-ec2}"
VPC_ID="${VPC_ID:-vpc-0e143b53ca2a4714c}"
SUBNET_ID="${SUBNET_ID:-subnet-02c1003124987322c}"
EIP_ALLOCATION_ID="${EIP_ALLOCATION_ID:-}"
ROOT_VOLUME_GB="${ROOT_VOLUME_GB:-50}"
DUPLI1_BRANCH="${DUPLI1_BRANCH:-main}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
USER_DATA_FILE="$(mktemp)"
trap 'rm -f "$USER_DATA_FILE"' EXIT

cat >"$USER_DATA_FILE" <<EOF
#!/bin/bash
set -euo pipefail
export DUPLI1_BRANCH=${DUPLI1_BRANCH}
export DUPLI1_AUTO_DEPLOY=1
curl -fsSL https://raw.githubusercontent.com/elug3/dupli1/${DUPLI1_BRANCH}/infra/scripts/ec2-bootstrap.sh -o /tmp/ec2-bootstrap.sh
chmod +x /tmp/ec2-bootstrap.sh
/tmp/ec2-bootstrap.sh >> /var/log/dupli1-bootstrap.log 2>&1
EOF

# Security group for the app host
SG_NAME="dupli1-ec2-sg"
SG_ID="$(aws ec2 describe-security-groups \
  --region "$AWS_REGION" \
  --filters "Name=group-name,Values=$SG_NAME" "Name=vpc-id,Values=$VPC_ID" \
  --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null || true)"

if [[ -z "$SG_ID" || "$SG_ID" == "None" ]]; then
  SG_ID="$(aws ec2 create-security-group \
    --region "$AWS_REGION" \
    --group-name "$SG_NAME" \
    --description "Dupli1 single EC2 app host" \
    --vpc-id "$VPC_ID" \
    --query GroupId --output text)"
  aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$SG_ID" --protocol tcp --port 22 --cidr 0.0.0.0/0
  aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$SG_ID" --protocol tcp --port 80 --cidr 0.0.0.0/0
  aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$SG_ID" --protocol tcp --port 8080 --cidr 0.0.0.0/0
  aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$SG_ID" --protocol tcp --port 443 --cidr 0.0.0.0/0
  echo "Created security group $SG_ID"
fi

# Key pair (create if missing; save PEM locally)
KEY_FILE="${SCRIPT_DIR}/../ec2/${KEY_NAME}.pem"
mkdir -p "$(dirname "$KEY_FILE")"
if ! aws ec2 describe-key-pairs --region "$AWS_REGION" --key-names "$KEY_NAME" >/dev/null 2>&1; then
  aws ec2 create-key-pair --region "$AWS_REGION" --key-name "$KEY_NAME" \
    --query KeyMaterial --output text > "$KEY_FILE"
  chmod 600 "$KEY_FILE"
  echo "Created key pair $KEY_NAME -> $KEY_FILE"
else
  echo "Using existing key pair $KEY_NAME (PEM at $KEY_FILE if you saved it earlier)"
fi

AMI_ID="$(aws ssm get-parameters \
  --region "$AWS_REGION" \
  --names /aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvn \
  --query 'Parameters[0].Value' --output text 2>/dev/null || true)"

if [[ -z "$AMI_ID" || "$AMI_ID" == "None" ]]; then
  AMI_ID="$(aws ec2 describe-images \
    --region "$AWS_REGION" \
    --owners 099720109477 \
    --filters "Name=name,Values=ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*" \
    --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)"
fi

echo "Launching $INSTANCE_TYPE with AMI $AMI_ID..."
INSTANCE_ID="$(aws ec2 run-instances \
  --region "$AWS_REGION" \
  --image-id "$AMI_ID" \
  --instance-type "$INSTANCE_TYPE" \
  --key-name "$KEY_NAME" \
  --subnet-id "$SUBNET_ID" \
  --security-group-ids "$SG_ID" \
  --associate-public-ip-address \
  --user-data "file://${USER_DATA_FILE}" \
  --block-device-mappings "[{\"DeviceName\":\"/dev/sda1\",\"Ebs\":{\"VolumeSize\":${ROOT_VOLUME_GB},\"VolumeType\":\"gp3\",\"DeleteOnTermination\":true}}]" \
  --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=${INSTANCE_NAME}}]" \
  --query 'Instances[0].InstanceId' --output text)"

echo "Waiting for instance $INSTANCE_ID to run..."
aws ec2 wait instance-running --region "$AWS_REGION" --instance-ids "$INSTANCE_ID"

# Allow this EC2 host to reach RDS for data migration
RDS_SG_ID="${RDS_SG_ID:-sg-073c0d32fa81e03e6}"
aws ec2 authorize-security-group-ingress \
  --region "$AWS_REGION" \
  --group-id "$RDS_SG_ID" \
  --protocol tcp \
  --port 5432 \
  --source-group "$SG_ID" 2>/dev/null \
  || echo "RDS ingress rule may already exist for $SG_ID"

if [[ -z "$EIP_ALLOCATION_ID" ]]; then
  EIP_ALLOCATION_ID="$(aws ec2 describe-addresses \
    --region "$AWS_REGION" \
    --filters Name=domain,Values=vpc \
    --query 'Addresses[?InstanceId==`null`].AllocationId | [0]' --output text)"
fi

if [[ -n "$EIP_ALLOCATION_ID" && "$EIP_ALLOCATION_ID" != "None" ]]; then
  echo "Associating Elastic IP $EIP_ALLOCATION_ID..."
  aws ec2 associate-address --region "$AWS_REGION" \
    --instance-id "$INSTANCE_ID" \
    --allocation-id "$EIP_ALLOCATION_ID" >/dev/null
fi

PUBLIC_IP="$(aws ec2 describe-instances --region "$AWS_REGION" --instance-ids "$INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)"

echo ""
echo "=== EC2 provisioned ==="
echo "Instance ID:  $INSTANCE_ID"
echo "Public IP:    $PUBLIC_IP"
echo "SSH:          ssh -i $KEY_FILE ubuntu@$PUBLIC_IP"
echo ""
echo "Bootstrap runs via user-data (~3-5 min). Then SSH in and:"
echo "  1. Edit /opt/dupli1/app/.env.prod (owner password, service accounts)"
echo "  2. bash /opt/dupli1/app/infra/scripts/deploy-ec2.sh"
echo "  3. bash /opt/dupli1/app/infra/scripts/migrate-rds-to-ec2.sh  # optional data import"
echo ""
echo "Point DNS at $PUBLIC_IP, then retire ALB/NAT/ECS/RDS when validated."
