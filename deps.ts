// Dependencies file for caching AWS SDK and other npm packages in the container
// This file is used during the Docker build process to pre-download dependencies

// AWS SDK v3 - S3 Client
export { S3Client, ListBucketsCommand, ListObjectsV2Command, GetObjectCommand, PutObjectCommand, DeleteObjectCommand } from "npm:@aws-sdk/client-s3@3.x";

// AWS SDK v3 - S3 Request Presigner (for generating presigned URLs)
export { getSignedUrl } from "npm:@aws-sdk/s3-request-presigner@3.x";

// Additional AWS SDK clients can be added here as needed
// export { DynamoDBClient } from "npm:@aws-sdk/client-dynamodb@3.x";
// export { SNSClient } from "npm:@aws-sdk/client-sns@3.x";
