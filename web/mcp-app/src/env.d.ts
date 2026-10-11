declare module "*.css";
declare module "*.svg?raw" {
  const svg: string;
  export default svg;
}
