#!/usr/bin/env -S deno run --allow-net --allow-env

/**
 * AWS S3 Presigned URLs Example for Deno
 * 
 * This example demonstrates how to correctly generate presigned URLs for
 * AWS S3 operations (upload and download) using AWS SDK v3.
 * 
 * Prerequisites:
 * - AWS credentials configured (via environment variables or AWS config)
 * - @aws-sdk/client-s3 package
 * - @aws-sdk/s3-request-presigner package
 */

import { S3Client, GetObjectCommand, PutObjectCommand } from "npm:@aws-sdk/client-s3@3";
import { getSignedUrl } from "npm:@aws-sdk/s3-request-presigner@3";

// Initialize S3 client
const client = new S3Client({ region: "us-west-2" });

// Example configuration
const BUCKET_NAME = "robust-cicada";
const OBJECT_KEY = "headshot.jpeg";
const URL_EXPIRATION_SECONDS = 86400; // 24 hours

async function generatePresignedUrls() {
  try {
    console.log("=== AWS S3 Presigned URLs Example ===\n");

    // 1. Generate presigned URL for UPLOAD (PUT operation)
    console.log("1. Generating presigned URL for upload...");
    const putCommand = new PutObjectCommand({
      Bucket: BUCKET_NAME,
      Key: OBJECT_KEY,
      ContentType: "image/jpeg", // Specify content type
    });

    const uploadUrl = await getSignedUrl(client, putCommand, {
      expiresIn: URL_EXPIRATION_SECONDS,
    });

    console.log("✅ Upload URL generated:");
    console.log(`   ${uploadUrl}\n`);

    // 2. Generate presigned URL for DOWNLOAD (GET operation)
    console.log("2. Generating presigned URL for download...");
    const getCommand = new GetObjectCommand({
      Bucket: BUCKET_NAME,
      Key: OBJECT_KEY,
    });

    const downloadUrl = await getSignedUrl(client, getCommand, {
      expiresIn: URL_EXPIRATION_SECONDS,
    });

    console.log("✅ Download URL generated:");
    console.log(`   ${downloadUrl}\n`);

    // 3. Example: Using the upload URL
    console.log("3. Example usage - Upload a file:");
    console.log(`
    // To upload using the presigned URL:
    const fileContent = await Deno.readFile("./local-file.jpeg");
    const uploadResponse = await fetch(uploadUrl, {
      method: "PUT",
      body: fileContent,
      headers: {
        "Content-Type": "image/jpeg",
      },
    });
    
    if (uploadResponse.ok) {
      console.log("File uploaded successfully!");
    }
    `);

    // 4. Example: Using the download URL
    console.log("4. Example usage - Download a file:");
    console.log(`
    // To download using the presigned URL:
    const downloadResponse = await fetch(downloadUrl);
    if (downloadResponse.ok) {
      const fileContent = await downloadResponse.arrayBuffer();
      await Deno.writeFile("./downloaded-file.jpeg", new Uint8Array(fileContent));
      console.log("File downloaded successfully!");
    }
    `);

    return { uploadUrl, downloadUrl };
  } catch (error) {
    console.error("❌ Error generating presigned URLs:", error);
    throw error;
  }
}

// Run the example if this file is executed directly
if (import.meta.main) {
  await generatePresignedUrls();
}

// Export for use in other modules
export { generatePresignedUrls };
