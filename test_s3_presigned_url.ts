// Test script for AWS S3 presigned URL generation
// This demonstrates the correct usage of AWS SDK v3 with the presigner

import { S3Client, GetObjectCommand, PutObjectCommand } from "npm:@aws-sdk/client-s3@3";
import { getSignedUrl } from "npm:@aws-sdk/s3-request-presigner@3";

console.log("Testing AWS S3 Presigned URL Generation...\n");

// Initialize S3 client
const client = new S3Client({ region: "us-west-2" });

try {
  // Generate a presigned URL for PUT operation (upload)
  console.log("1. Generating presigned URL for upload (PUT)...");
  const putCommand = new PutObjectCommand({
    Bucket: "robust-cicada",
    Key: "test/example-file.jpg",
    ContentType: "image/jpeg",
  });

  const uploadUrl = await getSignedUrl(client, putCommand, { expiresIn: 86400 }); // 24 hours
  console.log("✓ Upload URL generated successfully!");
  console.log(`  URL length: ${uploadUrl.length} characters`);
  console.log(`  Starts with: ${uploadUrl.substring(0, 50)}...`);
  console.log();

  // Generate a presigned URL for GET operation (download)
  console.log("2. Generating presigned URL for download (GET)...");
  const getCommand = new GetObjectCommand({
    Bucket: "robust-cicada",
    Key: "test/example-file.jpg",
  });

  const downloadUrl = await getSignedUrl(client, getCommand, { expiresIn: 86400 }); // 24 hours
  console.log("✓ Download URL generated successfully!");
  console.log(`  URL length: ${downloadUrl.length} characters`);
  console.log(`  Starts with: ${downloadUrl.substring(0, 50)}...`);
  console.log();

  console.log("✓ All tests passed!");
  console.log("\nKey takeaway: Use getSignedUrl(client, command, options) - it's a standalone function!");
  
} catch (error) {
  console.error("✗ Error generating signed URL:", error);
  console.error("\nCommon mistakes:");
  console.error("  - Using client.getSignedUrl() instead of getSignedUrl(client, ...)");
  console.error("  - Not importing getSignedUrl from @aws-sdk/s3-request-presigner");
  Deno.exit(1);
}
